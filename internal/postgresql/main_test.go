// Copyright 2025 Nibble-IT
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

package postgresql

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("moveDirRecursive", func() {
	var (
		randomString = func() string { return fmt.Sprintf("%x", rand.Int63()) }
		randomBytes  = func() []byte { return []byte(fmt.Sprintf("%x", rand.Int63())) }
	)
	When("moving a directory", func() {
		It("should successfully move", func() {
			const (
				walDir    = "wal"
				wal1      = walDir + "1"
				wal2      = walDir + "2"
				walSubDir = walDir + "/subdir"
				walDeep   = walDir + "/sub1/sub2"
			)
			for _, test := range []struct {
				src      string
				dst      string
				cleanSrc bool
				// preExisting is a dir (relative to src) holding a file, which is also an ancestor of dst
				preExisting string
			}{
				{src: wal1, dst: wal2, cleanSrc: true},
				{src: walDir, dst: walSubDir},
				{src: walSubDir, dst: walDir, cleanSrc: true},
				{src: walDir, dst: walDeep},
				{src: walDir, dst: walDeep + "/sub3", preExisting: "sub1"},
			} {
				var expected = map[string][]byte{}
				var unExpected []string
				fmt.Fprintf(GinkgoWriter, "DEBUG - Test: %v\n", test)
				tempDir, err := os.MkdirTemp("", "moveDirRecursive")
				Ω(err).NotTo(HaveOccurred())
				defer os.RemoveAll(tempDir) // Clean up after test
				srcDir := filepath.Join(tempDir, test.src)
				err = os.MkdirAll(srcDir, uRWX)
				Ω(err).NotTo(HaveOccurred())
				dstDir := filepath.Join(tempDir, test.dst)
				for _, subDir := range []string{
					"",
					randomString(),
					filepath.Join(randomString(), randomString()),
				} {
					if subDir != "" {
						unExpected = append(unExpected, filepath.Join(srcDir, subDir))
					}
					err = os.MkdirAll(filepath.Join(srcDir, subDir), uRWX)
					Ω(err).NotTo(HaveOccurred())
					for i := 0; i < 10; i++ {
						filePath := filepath.Join(subDir, randomString())
						data := randomBytes()
						expected[filepath.Join(dstDir, filePath)] = data
						err = os.WriteFile(filepath.Join(srcDir, filePath), data, uRW)
						Ω(err).NotTo(HaveOccurred())
					}
				}
				if test.preExisting != "" {
					filePath := filepath.Join(test.preExisting, randomString())
					data := randomBytes()
					err = os.MkdirAll(filepath.Join(srcDir, test.preExisting), uRWX)
					Ω(err).NotTo(HaveOccurred())
					err = os.WriteFile(filepath.Join(srcDir, filePath), data, uRW)
					Ω(err).NotTo(HaveOccurred())
					expected[filepath.Join(dstDir, filePath)] = data
					unExpected = append(unExpected, filepath.Join(srcDir, filePath))
				}
				err = moveDir(context.Background(), srcDir, dstDir)
				Ω(err).NotTo(HaveOccurred())
				for path, content := range expected {
					data, err := os.ReadFile(path)
					Ω(err).NotTo(HaveOccurred())
					Ω(data).To(Equal(content))
				}
				for _, path := range unExpected {
					_, err := os.Stat(path)
					Ω(err).To(HaveOccurred())
					Ω(err).To(MatchError(os.ErrNotExist))
				}
				if test.cleanSrc {
					_, err := os.Stat(srcDir)
					Ω(err).To(HaveOccurred())
					Ω(err).To(MatchError(os.ErrNotExist))
				}
			}
		})
		It("should fail soon", func() {
		})
	})
})

var _ = Describe("validateWalDir", func() {
	var (
		base    string
		dataDir string
		walDir  string
	)
	BeforeEach(func() {
		// On macOS TempDir itself sits below a symlink (/var -> /private/var)
		base = GinkgoT().TempDir()
		dataDir = filepath.Join(base, "data")
		walDir = filepath.Join(base, "wal")
		Ω(os.MkdirAll(dataDir, 0o700)).To(Succeed())
		Ω(os.MkdirAll(walDir, 0o700)).To(Succeed())
		// dataLink -> data and walLink -> data/pg_wal
		Ω(os.Symlink(dataDir, filepath.Join(base, "dataLink"))).To(Succeed())
	})
	DescribeTable("with pg_wal as a directory",
		func(rel string, viaDataLink bool, valid bool) {
			Ω(os.MkdirAll(filepath.Join(dataDir, "pg_wal", "sub"), 0o700)).To(Succeed())
			Ω(os.Symlink(filepath.Join(dataDir, "pg_wal"), filepath.Join(base, "walLink"))).To(Succeed())
			dd := dataDir
			if viaDataLink {
				dd = filepath.Join(base, "dataLink")
			}
			wd := ""
			if rel != "" {
				wd = filepath.Join(base, rel)
			}
			err := validateWalDir(dd, wd)
			if valid {
				Ω(err).NotTo(HaveOccurred())
			} else {
				Ω(err).To(HaveOccurred())
			}
		},
		Entry("empty", "", false, true),
		Entry("outside PGDATA", "wal", false, true),
		Entry("non-existing outside PGDATA", "wal/new/sub", false, true),
		Entry("sibling with common prefix", "data/pg_wal_new", false, true),
		Entry("pg_wal itself", "data/pg_wal", false, false),
		Entry("pg_wal with trailing slash", "data/pg_wal/", false, false),
		Entry("inside pg_wal", "data/pg_wal/sub", false, false),
		Entry("non-existing deep inside pg_wal", "data/pg_wal/sub1/../sub2/x", false, false),
		Entry("pg_wal via symlinked data dir", "data/pg_wal", true, false),
		Entry("walDir via symlinked parent", "dataLink/pg_wal/sub", false, false),
		Entry("walDir via symlink to pg_wal", "walLink/new", false, false),
	)
	It("accepts walDir as target of an existing pg_wal symlink", func() {
		Ω(os.Symlink(walDir, filepath.Join(dataDir, "pg_wal"))).To(Succeed())
		Ω(validateWalDir(dataDir, walDir)).To(Succeed())
		Ω(validateWalDir(filepath.Join(base, "dataLink"), filepath.Join(walDir, "sub"))).To(Succeed())
	})
	It("rejects pg_wal itself when it is a symlink", func() {
		Ω(os.Symlink(walDir, filepath.Join(dataDir, "pg_wal"))).To(Succeed())
		Ω(validateWalDir(dataDir, filepath.Join(dataDir, "pg_wal"))).NotTo(Succeed())
		Ω(validateWalDir(dataDir, filepath.Join(base, "dataLink", "pg_wal", "sub"))).NotTo(Succeed())
	})
})
