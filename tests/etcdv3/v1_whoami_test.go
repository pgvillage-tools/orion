package etcdv3_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net/http"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/auth"
	"github.com/pgvillage-tools/orion/internal/pki"
	client "github.com/pgvillage-tools/orion/pkg/api_client"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

var _ = Describe("GET /v1/whoami", func() {
	// clientWith returns an api client for the shared cluster that trusts its CA, with certificates as the client
	// certificates
	clientWith := func(ctx context.Context, certificates []tls.Certificate) client.Connection {
		tlsConfig, err := pki.CLIFiles(sharedCluster.PKI.CLIDir).ClientConfig()
		Ω(err).NotTo(HaveOccurred())
		tlsConfig.Certificates = certificates
		port, err := sharedCluster.API.MappedPort(ctx, fmt.Sprintf("%d/tcp", helpers.APIInternalPort))
		Ω(err).NotTo(HaveOccurred())
		return client.NewTLSConnection(helpers.LocalHost, port.Num(), helpers.APITimeout, tlsConfig)
	}

	// signClient signs a client certificate for subject with the api-clients intermediate of the shared cluster (the CA
	// that the api trusts for clients), valid from notBefore until notAfter
	signClient := func(subject pkix.Name, notBefore, notAfter time.Time) tls.Certificate {
		imDir := filepath.Join(sharedCluster.PKI.CLIDir, pki.CLICADir, pki.APIClientIntermediate)
		im, err := tls.LoadX509KeyPair(filepath.Join(imDir, "ca.crt"), filepath.Join(imDir, "ca.key"))
		Ω(err).NotTo(HaveOccurred())
		imCert, err := x509.ParseCertificate(im.Certificate[0])
		Ω(err).NotTo(HaveOccurred())
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		Ω(err).NotTo(HaveOccurred())
		serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
		Ω(err).NotTo(HaveOccurred())
		template := &x509.Certificate{
			SerialNumber: serial,
			Subject:      subject,
			NotBefore:    notBefore,
			NotAfter:     notAfter,
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		}
		der, err := x509.CreateCertificate(rand.Reader, template, imCert, &key.PublicKey, im.PrivateKey)
		Ω(err).NotTo(HaveOccurred())
		return tls.Certificate{Certificate: [][]byte{der, im.Certificate[0]}, PrivateKey: key}
	}

	It("returns the identity of the client certificate", func() {
		identity, httpCode, err := sharedCluster.APIClient.WhoAmI()
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
	It("returns the name and the groups of another client certificate of the same CA", func(ctx context.Context) {
		cert := signClient(pkix.Name{CommonName: "alice", OrganizationalUnit: []string{"dba", "ops"}},
			time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
		identity, httpCode, err := clientWith(ctx, []tls.Certificate{cert}).WhoAmI()
		Ω(err).NotTo(HaveOccurred())
		Ω(httpCode).To(Equal(http.StatusOK))
		Ω(identity.Name).To(Equal("alice"))
		Ω(identity.Groups).To(ConsistOf("dba", "ops"))
		Ω(identity.Method).To(Equal(auth.MethodClientCert))
	})
	It("refuses an expired client certificate", func(ctx context.Context) {
		cert := signClient(pkix.Name{CommonName: "expired"},
			time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))
		_, _, err := clientWith(ctx, []tls.Certificate{cert}).WhoAmI()
		Ω(err).To(MatchError(ContainSubstring("expired certificate")))
	})
	It("refuses the server certificate of the api as a client certificate", func(ctx context.Context) {
		// it is signed by the server intermediate, which the api doesn't trust for clients
		server, err := tls.LoadX509KeyPair(filepath.Join(sharedCluster.PKI.APIDir, pki.APIServerCertFile),
			filepath.Join(sharedCluster.PKI.APIDir, pki.APIServerKeyFile))
		Ω(err).NotTo(HaveOccurred())
		tlsConfig, err := pki.CLIFiles(sharedCluster.PKI.CLIDir).ClientConfig()
		Ω(err).NotTo(HaveOccurred())
		// A Go client doesn't send a certificate that isn't signed by a CA the server asks for; send it anyway, so
		// the api has to refuse it
		tlsConfig.Certificates = nil
		tlsConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return &server, nil
		}
		port, err := sharedCluster.API.MappedPort(ctx, fmt.Sprintf("%d/tcp", helpers.APIInternalPort))
		Ω(err).NotTo(HaveOccurred())
		_, _, err = client.NewTLSConnection(helpers.LocalHost, port.Num(), helpers.APITimeout, tlsConfig).WhoAmI()
		Ω(err).To(MatchError(ContainSubstring("unknown certificate authority")))
	})
})

var _ = Describe("GET /v1/whoami without the store", Ordered, func() {
	// this test pauses etcd, so it gets its own api
	tc := helpers.WithOwnAPI()

	It("still returns the identity while the store doesn't respond: whoami doesn't use the store",
		func(ctx context.Context) {
			helpers.PauseContainer(ctx, tc.Etcd)
			identity, httpCode, err := tc.APIClient.WhoAmI()
			Ω(err).NotTo(HaveOccurred())
			Ω(httpCode).To(Equal(http.StatusOK))
			Ω(identity.Name).To(Equal(pki.CLIClientName))
		})
})
