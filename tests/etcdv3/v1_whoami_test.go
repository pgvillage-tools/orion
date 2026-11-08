package etcdv3_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/pki"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
)

var _ = Describe("GET /v1/whoami", func() {
	// clientWith returns an api client for the shared cluster that trusts its CA, with certificates as the client
	// certificates
	clientWith := func(ctx context.Context, certificates []tls.Certificate) client.Connection {
		tlsConfig, err := pki.CLIFiles(sharedCluster.pki.CLIDir).ClientConfig()
		Ω(err).NotTo(HaveOccurred())
		tlsConfig.Certificates = certificates
		port, err := sharedCluster.api.MappedPort(ctx, fmt.Sprintf("%d/tcp", apiInternalPort))
		Ω(err).NotTo(HaveOccurred())
		return client.NewTLSConnection(localHost, port.Num(), apiTimeout, tlsConfig)
	}

	It("returns the identity of the client certificate", func() {
		identity, httpCode, err := sharedCluster.apiClient.WhoAmI()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(identity.Name).To(Equal(pki.CLIClientName))
		Ω(identity.Method).To(Equal(auth.MethodClientCert))
	})
	It("refuses a client without a certificate", func(ctx context.Context) {
		_, _, err := clientWith(ctx, nil).WhoAmI()
		Ω(err).To(MatchError(ContainSubstring("certificate required")))
	})
	It("refuses a client certificate of another CA", func(ctx context.Context) {
		other := pki.InitOptions{
			CLIDir: filepath.Join(GinkgoT().TempDir(), "cli"),
			APIDir: filepath.Join(GinkgoT().TempDir(), "api"),
		}
		_, err := pki.Init(other)
		Ω(err).NotTo(HaveOccurred())
		otherConfig, err := pki.CLIFiles(other.CLIDir).ClientConfig()
		Ω(err).NotTo(HaveOccurred())
		_, _, err = clientWith(ctx, otherConfig.Certificates).WhoAmI()
		Ω(err).To(MatchError(ContainSubstring("unknown certificate authority")))
	})
})
