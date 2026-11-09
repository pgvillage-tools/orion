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

package postgresql

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseConnString", func() {
	DescribeTable("parses valid connection strings",
		func(in string, expected ConnParams) {
			cp, err := ParseConnString(in)
			Ω(err).NotTo(HaveOccurred())
			Ω(cp).To(Equal(expected))
		},
		Entry("empty", "", ConnParams{}),
		Entry("only spaces", "   \t ", ConnParams{}),
		Entry("spaces around =", "host = myhost  port =5432", ConnParams{"host": "myhost", "port": "5432"}),
		Entry("empty value at the end", "host=myhost user=", ConnParams{"host": "myhost", "user": ""}),
		Entry("empty value with trailing spaces", "user=   ", ConnParams{"user": ""}),
		Entry("escaped space", `password=a\ b`, ConnParams{"password": "a b"}),
		Entry("quoted value", `password='a b' user=me`, ConnParams{"password": "a b", "user": "me"}),
		Entry("quoted value with escapes", `password='it\'s \\ ok'`, ConnParams{"password": `it's \ ok`}),
		Entry("empty quoted value", `password=''`, ConnParams{"password": ""}),
	)
	DescribeTable("rejects invalid connection strings",
		func(in string, expectedErr string) {
			cp, err := ParseConnString(in)
			Ω(err).To(MatchError(ContainSubstring(expectedErr)))
			Ω(cp).To(BeNil())
		},
		Entry("key without =", "host", `missing "=" after "host"`),
		Entry("key followed by spaces only", "host   ", `missing "=" after "host"`),
		Entry("key followed by another key", "host port=1", `missing "=" after "host"`),
		Entry("trailing backslash", `password=abc\`, "missing character after backslash"),
		Entry("unterminated quote", `password='abc`, "unterminated quoted string literal"),
	)
})

var _ = Describe("URLToConnParams", func() {
	It("parses query parameters and urls without user, port or path", func() {
		cp, err := URLToConnParams("postgres://myhost?sslmode=disable&application_name=app")
		Ω(err).NotTo(HaveOccurred())
		Ω(cp).To(Equal(ConnParams{
			ConnParamKeyHost:    "myhost",
			ConnParamKeySSLMode: "disable",
			ConnParamKeyAppName: "app",
		}))
	})
	DescribeTable("rejects invalid urls",
		func(in string, expectedErr string) {
			cp, err := URLToConnParams(in)
			Ω(err).To(MatchError(ContainSubstring(expectedErr)))
			Ω(cp).To(BeNil())
		},
		Entry("unparsable url", "postgres://my host:x/", "invalid"),
		Entry("other protocol", "mysql://myhost/db", "invalid connection protocol: mysql"),
	)
})

var _ = Describe("PGLsnToInt", func() {
	DescribeTable("converts valid lsns",
		func(in string, expected uint64) {
			Ω(PGLsnToInt(in)).To(Equal(expected))
		},
		Entry("zero", "0/0", uint64(0)),
		Entry("low part only", "0/5000090", uint64(0x5000090)),
		Entry("high part", "1/0", uint64(1)<<32),
		Entry("both parts", "FF/FFFFFFFF", uint64(0xFFFFFFFFFF)),
	)
	DescribeTable("rejects invalid lsns",
		func(in string) {
			_, err := PGLsnToInt(in)
			Ω(err).To(HaveOccurred())
		},
		Entry("no slash", "05000090"),
		Entry("too many parts", "0/1/2"),
		Entry("invalid high part", "zz/0"),
		Entry("invalid low part", "0/zz"),
		Entry("high part overflow", "100000000/0"),
	)
})

var _ = Describe("parseTimelinesHistory", func() {
	It("parses multiple lines and skips lines that do not match", func() {
		tlsh, err := parseTimelinesHistory("1\t0/5000090\tno recovery target specified\n\njunk\n2\t1/0\tbefore 2026\n")
		Ω(err).NotTo(HaveOccurred())
		Ω(tlsh).To(Equal([]*TimelineHistory{
			{TimelineID: 1, SwitchPoint: 0x5000090, Reason: "no recovery target specified"},
			{TimelineID: 2, SwitchPoint: uint64(1) << 32, Reason: "before 2026"},
		}))
	})
	DescribeTable("rejects invalid lines",
		func(in string, expectedErr string) {
			tlsh, err := parseTimelinesHistory(in)
			Ω(err).To(MatchError(ContainSubstring(expectedErr)))
			Ω(tlsh).To(BeNil())
		},
		Entry("invalid timeline id", "x\t0/5000090\treason", "cannot parse timelineID"),
		Entry("invalid lsn", "1\tbad\treason", "cannot parse start lsn"),
	)
})

var _ = Describe("WAL file names", func() {
	DescribeTable("IsWalFileName",
		func(name string, valid bool) {
			Ω(IsWalFileName(name)).To(Equal(valid))
		},
		Entry("valid", "000000010000000000000003", true),
		Entry("lower case", "00000001000000000000000a", false),
		Entry("too long", "0000000100000000000000030", false),
		Entry("history file", "00000002.history", false),
	)
	DescribeTable("XlogPosToWalFileNameNoTimeline",
		func(pos uint64, expected string) {
			Ω(XlogPosToWalFileNameNoTimeline(pos)).To(Equal(expected))
		},
		Entry("start", uint64(0), "0000000000000000"),
		Entry("within the first segment", uint64(walSegSize-1), "0000000000000000"),
		Entry("second segment", uint64(walSegSize), "0000000000000001"),
		Entry("high part", uint64(1)<<32|3*walSegSize+5, "0000000100000003"),
	)
	It("WalFileNameNoTimeLine rejects invalid names", func() {
		out, err := WalFileNameNoTimeLine("00000002.history")
		Ω(err).To(MatchError("bad wal file name"))
		Ω(out).To(BeEmpty())
	})
})

var _ = Describe("versions", func() {
	It("parses the output of postgres -V", func() {
		v, err := parseBinaryVersion("postgres (PostgreSQL) 16.4 (Debian 16.4-1)\n")
		Ω(err).NotTo(HaveOccurred())
		Ω(v.String()).To(Equal("16.4.0"))
	})
	It("rejects unexpected postgres -V output", func() {
		_, err := parseBinaryVersion("something else")
		Ω(err).To(MatchError(ContainSubstring("failed to parse postgres binary version")))
	})
	It("fails on a data dir without PG_VERSION", func() {
		_, err := pgDataVersion(GinkgoT().TempDir())
		Ω(err).To(MatchError(ContainSubstring("failed to read PG_VERSION")))
	})
})

var _ = Describe("filesystem helpers", func() {
	var base string
	BeforeEach(func() {
		base = GinkgoT().TempDir()
	})
	writeFile := func(path string, content string) {
		Ω(os.MkdirAll(filepath.Dir(path), uRWX)).To(Succeed())
		Ω(os.WriteFile(path, []byte(content), uRW)).To(Succeed())
	}

	Context("fileExists", func() {
		It("reports existing and missing paths", func() {
			writeFile(filepath.Join(base, "file"), "x")
			Ω(fileExists(filepath.Join(base, "file"))).To(BeTrue())
			Ω(fileExists(base)).To(BeTrue())
			Ω(fileExists(filepath.Join(base, "missing"))).To(BeFalse())
		})
		It("returns other stat errors", func() {
			writeFile(filepath.Join(base, "file"), "x")
			// a path below a regular file gives ENOTDIR, which is not a "not exist" error
			exists, err := fileExists(filepath.Join(base, "file", "sub"))
			Ω(err).To(HaveOccurred())
			Ω(exists).To(BeFalse())
		})
	})

	Context("isDirEmpty", func() {
		It("reports empty and non-empty dirs", func() {
			Ω(isDirEmpty(base)).To(BeTrue())
			writeFile(filepath.Join(base, "file"), "x")
			Ω(isDirEmpty(base)).To(BeFalse())
		})
		It("fails on a missing dir", func() {
			_, err := isDirEmpty(filepath.Join(base, "missing"))
			Ω(err).To(MatchError(os.ErrNotExist))
		})
		It("fails on a file", func() {
			writeFile(filepath.Join(base, "file"), "x")
			_, err := isDirEmpty(filepath.Join(base, "file"))
			Ω(err).To(HaveOccurred())
		})
	})

	Context("removeDirs", func() {
		It("removes empty dirs in order", func() {
			Ω(os.MkdirAll(filepath.Join(base, "a", "b"), uRWX)).To(Succeed())
			Ω(removeDirs([]string{filepath.Join(base, "a", "b"), filepath.Join(base, "a")})).To(Succeed())
			Ω(filepath.Join(base, "a")).NotTo(BeADirectory())
		})
		It("refuses to remove a dir that is not empty", func() {
			writeFile(filepath.Join(base, "a", "file"), "x")
			Ω(removeDirs([]string{filepath.Join(base, "a")})).To(MatchError(ContainSubstring("is not empty")))
			Ω(filepath.Join(base, "a", "file")).To(BeARegularFile())
		})
		It("fails on a missing dir", func() {
			Ω(removeDirs([]string{filepath.Join(base, "missing")})).To(MatchError(os.ErrNotExist))
		})
	})

	Context("moveFile", func() {
		It("renames a file", func() {
			src, dst := filepath.Join(base, "src"), filepath.Join(base, "dst")
			writeFile(src, "content")
			Ω(moveFile(src, dst)).To(Succeed())
			Ω(src).NotTo(BeAnExistingFile())
			Ω(os.ReadFile(dst)).To(Equal([]byte("content")))
		})
		It("fails when the source does not exist", func() {
			Ω(moveFile(filepath.Join(base, "missing"), filepath.Join(base, "dst"))).
				To(MatchError(ContainSubstring("Couldn't open source file")))
		})
		It("fails when rename and creating the destination both fail", func() {
			src := filepath.Join(base, "src")
			writeFile(src, "content")
			writeFile(filepath.Join(base, "dst", "file"), "x")
			Ω(moveFile(src, filepath.Join(base, "dst"))).NotTo(Succeed())
			Ω(src).To(BeARegularFile())
		})
		It("falls back to copying when renaming is not allowed", func() {
			if os.Geteuid() == 0 {
				Skip("permissions are not enforced for root")
			}
			srcDir := filepath.Join(base, "ro")
			src, dst := filepath.Join(srcDir, "src"), filepath.Join(base, "dst")
			writeFile(src, "content")
			// without write permission on its dir, src can be read but not renamed or removed
			Ω(os.Chmod(srcDir, 0o500)).To(Succeed())
			DeferCleanup(os.Chmod, srcDir, os.FileMode(uRWX))
			Ω(moveFile(src, dst)).To(MatchError(ContainSubstring("Failed removing original file")))
			Ω(os.ReadFile(dst)).To(Equal([]byte("content")))
		})
	})

	Context("moveDir", func() {
		ctx := context.Background()
		It("moves a single file", func() {
			src, dst := filepath.Join(base, "src"), filepath.Join(base, "dst")
			writeFile(src, "content")
			Ω(moveDir(ctx, src, dst)).To(Succeed())
			Ω(os.ReadFile(dst)).To(Equal([]byte("content")))
		})
		It("does nothing when src and dst are the same", func() {
			writeFile(filepath.Join(base, "src", "file"), "content")
			Ω(moveDir(ctx, filepath.Join(base, "src"), filepath.Join(base, "src", "."))).To(Succeed())
			Ω(filepath.Join(base, "src", "file")).To(BeARegularFile())
		})
		It("creates dst for an empty src", func() {
			Ω(os.Mkdir(filepath.Join(base, "src"), uRWX)).To(Succeed())
			Ω(moveDir(ctx, filepath.Join(base, "src"), filepath.Join(base, "dst"))).To(Succeed())
			Ω(filepath.Join(base, "dst")).To(BeADirectory())
			Ω(filepath.Join(base, "src")).NotTo(BeAnExistingFile())
		})
		It("fails when src does not exist", func() {
			Ω(moveDir(ctx, filepath.Join(base, "missing"), filepath.Join(base, "dst"))).
				To(MatchError(os.ErrNotExist))
		})
		It("fails when a file blocks a dir in dst", func() {
			writeFile(filepath.Join(base, "src", "sub", "file"), "content")
			writeFile(filepath.Join(base, "dst", "sub"), "blocking")
			Ω(moveDir(ctx, filepath.Join(base, "src"), filepath.Join(base, "dst"))).NotTo(Succeed())
		})
	})

	Context("isSubPath", func() {
		DescribeTable("compares paths",
			func(parent, child string, expected bool) {
				Ω(isSubPath(parent, child)).To(Equal(expected))
			},
			Entry("child", "/a", "/a/b", true),
			Entry("deep child", "/a", "/a/b/c", true),
			Entry("same", "/a", "/a", false),
			Entry("parent", "/a/b", "/a", false),
			Entry("sibling with common prefix", "/a", "/ab", false),
			Entry("relative and absolute", "a", "/a/b", false),
		)
	})
})
