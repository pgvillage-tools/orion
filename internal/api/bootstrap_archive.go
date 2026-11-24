// Copyright 2026 PgVillage
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"crypto/hpke"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/pkg/bootstrap"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

// The archive, its hand-over and the confirmation: see docs/api/bootstrap-design.md, "The archive"

// maxExtraSANs is the maximum number of extra names the cli can send for the api certificate
const maxExtraSANs = 32

// detectedSANs returns the names and addresses of the api host for its certificate: the hostname, the addresses of
// its interfaces, and the host of --base-url (when it is one)
func detectedSANs(baseURL string) []string {
	var sans []string
	if host, err := os.Hostname(); err == nil && oriontls.IsValidServer(host) {
		sans = append(sans, host)
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLinkLocalUnicast() {
				sans = append(sans, ipNet.IP.String())
			}
		}
	}
	if ip := net.ParseIP(baseURL); baseURL != "" && (ip == nil || !ip.IsUnspecified()) &&
		oriontls.IsValidServer(baseURL) {
		sans = append(sans, baseURL)
	}
	return sans
}

// archive generates the certificates and the private keys on the proven TLS connection with keying material ekm,
// and returns the archive: the private keys encrypted to pub. The session keeps them until the cli stored them.
func (b *bootstrapper) archive(ekm []byte, pub hpke.PublicKey, sans []string) (endpoints.BootstrapArchive,
	*bootstrapError) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	now := b.now()
	b.prune(now)
	if err := b.check(now); err != nil {
		return endpoints.BootstrapArchive{}, err
	}
	session, err := b.provenSession(ekm)
	if err != nil {
		return endpoints.BootstrapArchive{}, err
	}
	// a new archive (with a new CA) every time: until the cli stored one, nothing trusts it
	chain, genErr := pki.Generate(pki.InitOptions{APISANs: append(append([]string{}, b.sans...), sans...)})
	if genErr != nil {
		return endpoints.BootstrapArchive{}, &bootstrapError{status: http.StatusInternalServerError,
			detail: "failed to create the certificates: " + genErr.Error()}
	}
	certs, keys := pki.ArchiveContents(*chain)
	archive, sealErr := bootstrap.SealArchive(pub, ekm, certs, keys)
	if sealErr != nil {
		return endpoints.BootstrapArchive{}, &bootstrapError{status: http.StatusInternalServerError,
			detail: "failed to encrypt the archive: " + sealErr.Error()}
	}
	session.chain, session.archive = chain, &archive
	return archive, nil
}

// provenSession returns the session of the TLS connection with keying material ekm, when the cli proved it has the
// secret on it; the caller holds the mutex
func (b *bootstrapper) provenSession(ekm []byte) (*bootstrapSession, *bootstrapError) {
	session, ok := b.sessions[string(ekm)]
	if !ok || !session.proven {
		return nil, &bootstrapError{status: http.StatusConflict,
			detail: "no bootstrap proof on this connection: post to " + string(endpoints.BootstrapChallengeEndPoint) +
				" and " + string(endpoints.BootstrapProofEndPoint) + " first, on the same TLS connection"}
	}
	return session, nil
}

// stored stores the files of the api and the archive of the TLS connection with keying material ekm in the api
// directory, after the cli stored the archive: the secret is used up then
func (b *bootstrapper) stored(ekm []byte) *bootstrapError {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	now := b.now()
	b.prune(now)
	if err := b.check(now); err != nil {
		return err
	}
	session, bErr := b.provenSession(ekm)
	if bErr != nil {
		return bErr
	}
	if session.chain == nil {
		return &bootstrapError{status: http.StatusConflict,
			detail: "no archive on this connection: post to " + string(endpoints.BootstrapArchiveEndPoint) + " first"}
	}
	data, err := json.Marshal(session.archive)
	if err == nil {
		// the archive first: when it is there, the api files are from it
		err = oriontls.WriteFile(filepath.Join(b.dir, pki.APIBootstrapArchiveFile), data, true)
	}
	if err == nil {
		err = pki.WriteAPIFiles(*session.chain, b.dir)
	}
	if err != nil {
		return &bootstrapError{status: http.StatusInternalServerError,
			detail: "failed to store the certificate of the api: " + err.Error()}
	}
	b.usedUp = true
	clear(b.sessions)
	return nil
}

// BootstrapArchiveHandler returns the archive on a proven TLS connection: the certificates, and the private keys
// encrypted to the public key of the cli
func (h *Handlers) BootstrapArchiveHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	ekm, err := bootstrap.ExportKeyingMaterial(r.TLS)
	if err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req endpoints.BootstrapArchiveRequest
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBootstrapBody)).Decode(&req); err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "invalid bootstrap archive request")
		return
	}
	pub, err := bootstrap.KEM().NewPublicKey(req.PublicKey)
	if err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, "invalid bootstrap public key: "+err.Error())
		return
	}
	if len(req.SANs) > maxExtraSANs {
		endpoints.WriteProblem(w, http.StatusBadRequest, fmt.Sprintf("at most %d extra names", maxExtraSANs))
		return
	}
	for _, san := range req.SANs {
		if !oriontls.IsValidServer(san) {
			endpoints.WriteProblem(w, http.StatusBadRequest, fmt.Sprintf("invalid name or address %q", san))
			return
		}
	}
	archive, bErr := h.bootstrap.archive(ekm, pub, req.SANs)
	if bErr != nil {
		logger.Warn().Str("client", r.RemoteAddr).Str("reason", bErr.detail).Msg("refused a bootstrap archive")
		writeBootstrapError(w, bErr)
		return
	}
	logger.Info().Str("client", r.RemoteAddr).Strs("sans", req.SANs).Msg("handed out a bootstrap archive")
	h.writeJSON(ctx, w, archive)
}

// BootstrapStoredHandler stores the certificate of the api after the cli stored the archive, and switches the api to
// normal mode
func (h *Handlers) BootstrapStoredHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	ekm, err := bootstrap.ExportKeyingMaterial(r.TLS)
	if err != nil {
		endpoints.WriteProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	if bErr := h.bootstrap.stored(ekm); bErr != nil {
		logger.Warn().Str("client", r.RemoteAddr).Str("reason", bErr.detail).Msg("refused a bootstrap stored")
		writeBootstrapError(w, bErr)
		return
	}
	logger.Info().Str("client", r.RemoteAddr).
		Msg("the cli stored the bootstrap archive: the secret is used up, switching to normal mode")
	h.writeJSON(ctx, w, endpoints.BootstrapStatusResponse{Status: endpoints.BootstrapStatusStored})
	h.finishBootstrap()
}

// BootstrapConfirmRoutes returns the route of the confirmation, in normal mode
func (h *Handlers) BootstrapConfirmRoutes() []Route {
	return []Route{
		{endpoints.BootstrapConfirmEndPoint.Route(endpoints.MethodPost), h.BootstrapConfirmHandler},
	}
}

// BootstrapConfirmHandler takes the confirmation of the cli, over mTLS with its new certificate, and deletes the
// archive; it can be repeated
func (h *Handlers) BootstrapConfirmHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, logger := logging.GetLogComponent(ctx, logging.WebApiComponent)
	identity, ok := auth.FromContext(ctx)
	if !ok || identity.Name != pki.CLIClientName {
		endpoints.WriteProblem(w, http.StatusForbidden, "only "+pki.CLIClientName+" can confirm the bootstrap")
		return
	}
	if h.archiveFile != "" {
		err := os.Remove(h.archiveFile)
		switch {
		case err == nil:
			logger.Info().Str("client", r.RemoteAddr).
				Msg("the cli confirmed the bootstrap: removed the archive with the CA keys")
		case !errors.Is(err, fs.ErrNotExist):
			handleError(ctx, err, w, "failed to remove the bootstrap archive")
			return
		}
	}
	h.writeJSON(ctx, w, endpoints.BootstrapStatusResponse{Status: endpoints.BootstrapStatusConfirmed})
}
