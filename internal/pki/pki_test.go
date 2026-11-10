package pki_test

import (
	"crypto/tls"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/pgvillage-tools/orion/internal/pki"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

var _ = Describe("PKI", Ordered, func() {
	var (
		tmpDir string
		opts   pki.InitOptions
		chain  *oriontls.Chain
	)
	BeforeAll(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "pkiTest")
		Expect(err).NotTo(HaveOccurred())
		opts = pki.InitOptions{
			CLIDir:  filepath.Join(tmpDir, "cli"),
			APIDir:  filepath.Join(tmpDir, "api"),
			APISANs: []string{"orion-api.example.org", "localhost"},
		}
		chain, err = pki.Init(opts)
		Expect(err).NotTo(HaveOccurred())
	})
	AfterAll(func() {
		Expect(os.RemoveAll(tmpDir)).To(Succeed())
	})

	Context("when initializing", func() {
		It("creates all files", func() {
			for _, f := range opts.Files() {
				Expect(f).To(BeAnExistingFile())
			}
		})
		It("makes private keys readable for the owner only", func() {
			for _, f := range opts.Files() {
				if filepath.Ext(f) != ".key" {
					continue
				}
				info, err := os.Stat(f)
				Expect(err).NotTo(HaveOccurred())
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)), f)
			}
		})
		It("doesn't give the api any CA key", func() {
			entries, err := os.ReadDir(opts.APIDir)
			Expect(err).NotTo(HaveOccurred())
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			Expect(names).To(ConsistOf(pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile))
		})
		It("puts the default and extra names on the api certificate", func() {
			server := chain.Intermediates[pki.ServerIntermediate].Children()[pki.APIServerName].Cert.X509()
			Expect(server.Subject.CommonName).To(Equal(pki.APIServerName))
			Expect(server.DNSNames).To(ConsistOf(pki.APIServerName, "localhost", "orion-api.example.org"))
			Expect(server.IPAddresses).To(HaveLen(2))
		})
		It("refuses to overwrite existing files", func() {
			_, err := pki.Init(opts)
			Expect(err).To(MatchError(ContainSubstring("refusing to overwrite")))
		})
		It("overwrites existing files when forced", func() {
			forced := opts
			forced.CLIDir = filepath.Join(tmpDir, "forced-cli")
			forced.APIDir = filepath.Join(tmpDir, "forced-api")
			_, err := pki.Init(forced)
			Expect(err).NotTo(HaveOccurred())
			forced.Force = true
			_, err = pki.Init(forced)
			Expect(err).NotTo(HaveOccurred())
		})
		It("refuses an api directory that contains the CA directory", func() {
			for _, apiDir := range []string{tmpDir, filepath.Join(tmpDir, "nested")} {
				_, err := pki.Init(pki.InitOptions{
					CLIDir: filepath.Join(tmpDir, "nested", "cli"), APIDir: apiDir, Force: true,
				})
				Expect(err).To(MatchError(ContainSubstring("contains the CA directory")), apiDir)
			}
			Expect(os.MkdirAll(filepath.Join(tmpDir, "nested"), 0o700)).To(Succeed())
			link := filepath.Join(tmpDir, "link-to-nested")
			Expect(os.Symlink(filepath.Join(tmpDir, "nested"), link)).To(Succeed())
			_, err := pki.Init(pki.InitOptions{
				CLIDir: filepath.Join(tmpDir, "nested", "cli"), APIDir: link, Force: true,
			})
			Expect(err).To(MatchError(ContainSubstring("contains the CA directory")))
		})
		It("validates the names for the api certificate before it writes anything", func() {
			before, err := os.ReadFile(filepath.Join(opts.CLIDir, pki.CLICADir, "root.crt"))
			Expect(err).NotTo(HaveOccurred())
			invalid := opts
			invalid.Force = true
			invalid.APISANs = []string{"not a name"}
			_, err = pki.Init(invalid)
			Expect(err).To(MatchError(ContainSubstring("invalid name or address")))
			after, err := os.ReadFile(filepath.Join(opts.CLIDir, pki.CLICADir, "root.crt"))
			Expect(err).NotTo(HaveOccurred())
			Expect(after).To(Equal(before))
		})
		It("needs both directories", func() {
			_, err := pki.Init(pki.InitOptions{CLIDir: tmpDir})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when connecting", func() {
		var server *httptest.Server
		BeforeAll(func() {
			serverConfig, err := pki.APIFiles(opts.APIDir).ServerConfig()
			Expect(err).NotTo(HaveOccurred())
			server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, r.TLS.PeerCertificates[0].Subject.CommonName)
			}))
			server.TLS = serverConfig
			server.StartTLS()
		})
		AfterAll(func() {
			server.Close()
		})
		get := func(config *tls.Config) (string, error) {
			client := http.Client{Transport: &http.Transport{TLSClientConfig: config}, Timeout: 5 * time.Second}
			resp, err := client.Get(server.URL)
			if err != nil {
				return "", err
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			return string(body), err
		}

		It("accepts the cli certificate, and the cli accepts the api", func() {
			clientConfig, err := pki.CLIFiles(opts.CLIDir).ClientConfig()
			Expect(err).NotTo(HaveOccurred())
			Expect(get(clientConfig)).To(Equal(pki.CLIClientName))
		})
		It("refuses a client without a certificate", func() {
			pool, _, err := pki.LoadCertPool(filepath.Join(opts.CLIDir, pki.CLICAFile))
			Expect(err).NotTo(HaveOccurred())
			_, err = get(&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13})
			Expect(err).To(HaveOccurred())
		})
		It("refuses a client certificate of another intermediate", func() {
			other := oriontls.Intermediate{Clients: []string{pki.CLIClientName}}
			Expect(other.InitializeIntermediate("etcd-clients", chain.Root)).To(Succeed())
			Expect(other.InitializeClients()).To(Succeed())
			pair := other.Children()[pki.CLIClientName]
			cert, err := tls.X509KeyPair(pair.Cert.PEM, pair.PrivateKey.PEM)
			Expect(err).NotTo(HaveOccurred())
			pool, _, err := pki.LoadCertPool(filepath.Join(opts.CLIDir, pki.CLICAFile))
			Expect(err).NotTo(HaveOccurred())
			_, err = get(&tls.Config{RootCAs: pool, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13})
			Expect(err).To(HaveOccurred())
		})
		It("refuses an api with a certificate of another CA", func() {
			otherDir := filepath.Join(tmpDir, "other")
			_, err := pki.Init(pki.InitOptions{
				CLIDir: filepath.Join(otherDir, "cli"), APIDir: filepath.Join(otherDir, "api"),
			})
			Expect(err).NotTo(HaveOccurred())
			clientConfig, err := pki.CLIFiles(filepath.Join(otherDir, "cli")).ClientConfig()
			Expect(err).NotTo(HaveOccurred())
			_, err = get(clientConfig)
			Expect(err).To(MatchError(ContainSubstring("unknown authority")))
		})
	})

	Context("when checking the expiry", func() {
		It("reports nothing for new certificates", func() {
			certs, err := pki.APIFiles(opts.APIDir).Certificates()
			Expect(err).NotTo(HaveOccurred())
			Expect(certs).To(HaveLen(3))
			Expect(pki.ExpiringCertificates(certs, time.Now(), pki.ExpiryWarning)).To(BeEmpty())
		})
		It("reports certificates that expire soon", func() {
			certs, err := pki.APIFiles(opts.APIDir).Certificates()
			Expect(err).NotTo(HaveOccurred())
			expiring := pki.ExpiringCertificates(certs, time.Now().Add(oriontls.Year), pki.ExpiryWarning)
			Expect(expiring).To(ContainElement(ContainSubstring(pki.APIServerName)))
		})
	})
})

var _ = Describe("Helpers", func() {
	It("parses validities", func() {
		for input, expected := range map[string]time.Duration{
			"5y":  5 * oriontls.Year,
			"30d": 30 * oriontls.Day,
			"36h": 36 * time.Hour,
		} {
			Expect(pki.ParseValidity(input)).To(Equal(expected), input)
		}
		for _, invalid := range []string{"", "0y", "-1d", "y", "5x", "-5h", "585y", "106752d"} {
			_, err := pki.ParseValidity(invalid)
			Expect(err).To(HaveOccurred(), invalid)
		}
	})
	It("expands the home directory", func() {
		home, err := os.UserHomeDir()
		Expect(err).NotTo(HaveOccurred())
		Expect(pki.ExpandHome("~/.orion/api")).To(Equal(filepath.Join(home, ".orion/api")))
		Expect(pki.ExpandHome("~")).To(Equal(home))
		Expect(pki.ExpandHome("/etc/orion")).To(Equal("/etc/orion"))
		Expect(pki.ExpandHome("~other/x")).To(Equal("~other/x"))
		files, err := pki.CLIFiles("~/cli").Expand()
		Expect(err).NotTo(HaveOccurred())
		Expect(files.CAFile).To(Equal(filepath.Join(home, "cli", pki.CLICAFile)))
	})
	It("reports missing files", func() {
		Expect(pki.APIFiles("/nonexistent").Exist()).To(BeFalse())
		_, err := pki.APIFiles("/nonexistent").ServerConfig()
		Expect(err).To(HaveOccurred())
	})
	Describe("Check", func() {
		var files pki.TLSFiles
		BeforeEach(func() {
			dir := GinkgoT().TempDir()
			files = pki.APIFiles(dir)
			for _, p := range []string{files.CertFile, files.KeyFile, files.CAFile} {
				Expect(os.WriteFile(p, []byte("x"), 0o600)).To(Succeed())
			}
		})
		It("accepts files it can read", func() {
			Expect(files.Check()).To(Succeed())
		})
		It("reports a missing file as not found, with its name", func() {
			Expect(os.Remove(files.KeyFile)).To(Succeed())
			err := files.Check()
			Expect(err).To(MatchError(pki.ErrFilesNotFound))
			Expect(err).To(MatchError(ContainSubstring(files.KeyFile)))
		})
		It("reports a file it can't read as such, not as not found", func() {
			if os.Getuid() == 0 {
				Skip("root reads every file")
			}
			Expect(os.Chmod(files.KeyFile, 0)).To(Succeed())
			err := files.Check()
			Expect(err).NotTo(MatchError(pki.ErrFilesNotFound))
			Expect(err).To(MatchError(fs.ErrPermission))
			Expect(err).To(MatchError(ContainSubstring("cannot read " + files.KeyFile)))
		})
	})
})
