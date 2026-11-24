package cli_test

import (
	"context"
	"crypto/x509"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/pki"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
	"github.com/pgvillage-tools/orion/tests/helpers"
)

// The path of a user who starts the api without certificates: the api starts in bootstrap mode, orion-cli bootstrap
// gets the certificates and the CA keys from it, and from then on the cli and the api use mTLS, also after the api
// restarts.
var _ = Describe("bootstrapping the api over TLS with orion-cli", Ordered, func() {
	// etcd only: the api starts without certificates
	tc := helpers.WithOwnStore()

	const secret = "orion-e2e-bootstrap-secret"
	// the certificates the api presented in bootstrap mode, and after the bootstrap
	var bootstrapCert, apiCert *x509.Certificate

	apiMode := func(ctx context.Context) string {
		mode, _, err := tc.ProbeClient.Mode()
		Ω(err).NotTo(HaveOccurred())
		return mode.Mode
	}

	It("starts the api in bootstrap mode when it has no certificates", func(ctx context.Context) {
		tc.RunAPIInBootstrapMode(ctx, secret)
		Ω(apiMode(ctx)).To(Equal(endpoints.ModeBootstrap))

		// a temporary self-signed certificate, which the cli can't verify (the secret authenticates the api instead)
		certs := tc.APIServerCertificates(ctx, false)
		Ω(certs).To(HaveLen(1))
		bootstrapCert = certs[0]
		Ω(bootstrapCert.Issuer.String()).To(Equal(bootstrapCert.Subject.String()))
		Ω(bootstrapCert.CheckSignature(bootstrapCert.SignatureAlgorithm, bootstrapCert.RawTBSCertificate,
			bootstrapCert.Signature)).To(Succeed())
	})
	It("doesn't bootstrap with a wrong secret", func(ctx context.Context) {
		out, code := tc.Bootstrap(ctx, "not-the-secret-of-the-api")
		Ω(code).To(Equal(1), out)
		Ω(out).To(ContainSubstring("the cli doesn't have the bootstrap secret of the api"))
		Ω(apiMode(ctx)).To(Equal(endpoints.ModeBootstrap))
	})
	It("hands the certificates and the CA keys over to the cli with the secret", func(ctx context.Context) {
		out, code := tc.Bootstrap(ctx, secret)
		Ω(code).To(Equal(0), out)
		Ω(out).To(ContainSubstring("The api proved that it has the bootstrap secret"))
		Ω(out).To(ContainSubstring("Stored the certificates and the CA keys in "))
		Ω(out).To(ContainSubstring("the bootstrap is done"))
		Ω(apiMode(ctx)).To(Equal(endpoints.ModeNormal))
	})
	It("presents the api certificate from the bootstrap, signed by the CA of the cli", func(ctx context.Context) {
		// verified against the CA the cli got in the bootstrap, for the name the cli connects to (--api-san)
		certs := tc.APIServerCertificates(ctx, true)
		Ω(len(certs)).To(BeNumerically(">=", 2), "the api certificate and its intermediate")
		apiCert = certs[0]
		Ω(apiCert.Equal(bootstrapCert)).To(BeFalse())
		Ω(apiCert.Subject.CommonName).To(Equal(pki.APIServerName))
		Ω(apiCert.IsCA).To(BeFalse())
		Ω(apiCert.ExtKeyUsage).To(ContainElement(x509.ExtKeyUsageServerAuth))
		Ω(apiCert.DNSNames).To(ContainElements("api", "localhost"))
		Ω(apiCert.IPAddresses).To(ContainElement(WithTransform(net.IP.String, Equal("127.0.0.1"))))
		Ω(certs[1].IsCA).To(BeTrue())
		Ω(apiCert.CheckSignatureFrom(certs[1])).To(Succeed())
	})
	It("connects to the api with the client certificate from the bootstrap", func(ctx context.Context) {
		out, code := tc.CLI(ctx, "connect")
		Ω(code).To(Equal(0), out)
		Ω(out).To(ContainSubstring("Connected to the api as orion-cli (client-certificate)"))
	})
	It("keeps its certificates when it restarts, and doesn't bootstrap again", func(ctx context.Context) {
		tc.RestartAPI(ctx, endpoints.ModeNormal)
		Ω(tc.APIServerCertificates(ctx, true)[0].Equal(apiCert)).To(BeTrue(), "the same api certificate")

		out, code := tc.CLI(ctx, "connect")
		Ω(code).To(Equal(0), out)

		out, code = tc.Bootstrap(ctx, secret)
		Ω(code).To(Equal(1), out)
		Ω(out).To(ContainSubstring("not in bootstrap mode"))
	})
	It("creates a cluster through the bootstrapped api", func(ctx context.Context) {
		out, code := tc.CLI(ctx, "init")
		Ω(code).To(Equal(0), out)

		spec, _, err := tc.APIClient.GetClusterSpec()
		Ω(err).NotTo(HaveOccurred())
		Ω(spec.InitMode).To(HaveValue(Equal(apiv1.New)))
	})
})
