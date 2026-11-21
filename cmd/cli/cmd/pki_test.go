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

package cmd

import (
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
	oriontls "github.com/pgvillage-tools/orion/pkg/tls"
)

var _ = Describe("pkiInitOptions", func() {
	It("converts the flags to the options of pki init", func() {
		home, err := os.UserHomeDir()
		Ω(err).NotTo(HaveOccurred())
		o := pkiInitOptions{
			apiDir:               "~/api",
			apiSANs:              []string{"orion-api.example.org"},
			keyAlgorithm:         string(oriontls.KeyAlgorithmRSA2048),
			rootValidity:         "10y",
			intermediateValidity: "1000d",
			leafValidity:         "720h",
		}
		opts, err := o.initOptions("/cli", "api.example.org")
		Ω(err).NotTo(HaveOccurred())
		Ω(opts.APIDir).To(Equal(filepath.Join(home, "api")))
		Ω(opts.CLIDir).To(Equal("/cli"))
		Ω(opts.APISANs).To(Equal([]string{"api.example.org", "orion-api.example.org"}))
		Ω(opts.RootExpiry).To(Equal(10 * oriontls.Year))
		Ω(opts.IntermediateExpiry).To(Equal(1000 * oriontls.Day))
		Ω(opts.LeafExpiry).To(Equal(720 * time.Hour))
		Ω(opts.KeyAlgorithm).To(Equal(oriontls.KeyAlgorithmRSA2048))

		o.leafValidity = "forever"
		_, err = o.initOptions("/cli", "localhost")
		Ω(err).To(HaveOccurred(), "expected an invalid validity to be refused")
	})
})

var _ = Describe("cliTLSFiles", func() {
	It("returns the files in --pki-dir", func() {
		dir := GinkgoT().TempDir()
		usePKIDir(dir)
		files, err := cliTLSFiles()
		Ω(err).NotTo(HaveOccurred())
		Ω(files).To(Equal(pki.CLIFiles(dir)))
	})
})
