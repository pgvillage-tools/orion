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
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/pki"
)

var _ = Describe("the bootstrap state", func() {
	var dir string

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
	})

	// write creates the files names in dir
	write := func(names ...string) {
		GinkgoHelper()
		for _, name := range names {
			Expect(os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600)).To(Succeed())
		}
	}
	// exist returns the files of names that exist in dir
	exist := func(names ...string) []string {
		var found []string
		for _, name := range names {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				found = append(found, name)
			}
		}
		return found
	}
	apiFiles := []string{pki.APIServerCertFile, pki.APIServerKeyFile, pki.APIClientCAFile}
	allFiles := append(append([]string{}, apiFiles...), pki.APIBootstrapArchiveFile, pki.APIBootstrapStateFile)

	Context("readBootstrapState", func() {
		It("is empty without a state file", func() {
			Expect(readBootstrapState(dir)).To(Equal(bootstrapState{}))
		})

		It("reads what writeBootstrapState wrote, only readable by the owner", func() {
			state := bootstrapState{State: stateBootstrapping, SecretSHA256: "abc", Expires: time.Unix(1e9, 0).UTC()}
			Expect(writeBootstrapState(dir, state)).To(Succeed())
			Expect(readBootstrapState(dir)).To(Equal(state))
			info, err := os.Stat(stateFile(dir))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
		})

		DescribeTable("refuses",
			func(content, want string) {
				Expect(os.WriteFile(stateFile(dir), []byte(content), 0o600)).To(Succeed())
				_, err := readBootstrapState(dir)
				Expect(err).To(MatchError(ContainSubstring(want)))
			},
			Entry("no JSON", "not json", "invalid bootstrap state"),
			Entry("an unknown state", `{"state":"lost"}`, `invalid bootstrap state "lost"`),
		)
	})

	Context("prepareBootstrap", func() {
		DescribeTable("with --rebootstrap removes the files of an unconfirmed bootstrap",
			func(ctx SpecContext, state string) {
				write(allFiles...)
				Expect(writeBootstrapState(dir, bootstrapState{State: state})).To(Succeed())
				Expect(prepareBootstrap(ctx, dir, true)).To(Equal(bootstrapState{}))
				Expect(exist(allFiles...)).To(BeEmpty())
			},
			Entry("bootstrapping", stateBootstrapping),
			Entry("handed over", stateHandedOver),
		)

		It("with --rebootstrap changes nothing after a confirmed bootstrap", func(ctx SpecContext) {
			write(apiFiles...)
			Expect(writeBootstrapState(dir, bootstrapState{State: stateConfirmed})).To(Succeed())
			Expect(prepareBootstrap(ctx, dir, true)).To(Equal(bootstrapState{State: stateConfirmed}))
			Expect(exist(apiFiles...)).To(Equal(apiFiles))
		})

		It("with --rebootstrap never removes your own certificates", func(ctx SpecContext) {
			write(apiFiles...)
			Expect(prepareBootstrap(ctx, dir, true)).To(Equal(bootstrapState{}))
			Expect(exist(apiFiles...)).To(Equal(apiFiles))
		})

		It("removes the files of a hand-over that was interrupted, and continues the bootstrap", func(ctx SpecContext) {
			write(pki.APIBootstrapArchiveFile, pki.APIServerCertFile)
			state := bootstrapState{State: stateBootstrapping, SecretSHA256: "abc"}
			Expect(writeBootstrapState(dir, state)).To(Succeed())
			Expect(prepareBootstrap(ctx, dir, false)).To(Equal(state))
			Expect(exist(allFiles...)).To(Equal([]string{pki.APIBootstrapStateFile}))
		})

		It("leaves certificates without an archive alone", func(ctx SpecContext) {
			write(apiFiles...)
			Expect(writeBootstrapState(dir, bootstrapState{State: stateBootstrapping})).To(Succeed())
			_, err := prepareBootstrap(ctx, dir, false)
			Expect(err).NotTo(HaveOccurred())
			Expect(exist(apiFiles...)).To(Equal(apiFiles))
		})

		It("returns an invalid state", func(ctx SpecContext) {
			Expect(os.WriteFile(stateFile(dir), []byte("x"), 0o600)).To(Succeed())
			_, err := prepareBootstrap(ctx, dir, false)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("resumeSecret", func() {
		now := time.Now()
		ttl := time.Hour
		generated := "GENERATEDSECRETOFTWENTYSIXX"
		valid := bootstrapState{State: stateBootstrapping, Secret: generated,
			SecretSHA256: secretHash([]byte(generated)), Expires: now.Add(time.Minute)}

		It("continues with a generated secret that is still valid", func(ctx SpecContext) {
			secret, expires, err := resumeSecret(ctx, valid, nil, ttl, now)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(secret)).To(Equal(generated))
			Expect(expires).To(Equal(valid.Expires))
		})

		It("continues with a set secret that is still valid, when it is the same one", func(ctx SpecContext) {
			state := bootstrapState{State: stateBootstrapping, SecretSHA256: secretHash([]byte(testSecret)),
				Expires: now.Add(time.Minute)}
			secret, expires, err := resumeSecret(ctx, state, []byte(testSecret), ttl, now)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(secret)).To(Equal(testSecret))
			Expect(expires).To(Equal(state.Expires))

			_, expires, err = resumeSecret(ctx, state, []byte("another secret of twenty characters"), ttl, now)
			Expect(err).NotTo(HaveOccurred())
			Expect(expires).To(Equal(now.Add(ttl)), "another secret starts over")
		})

		It("takes a set secret instead of a stored generated one", func(ctx SpecContext) {
			secret, expires, err := resumeSecret(ctx, valid, []byte(testSecret), ttl, now)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(secret)).To(Equal(testSecret))
			Expect(expires).To(Equal(now.Add(ttl)))
		})

		DescribeTable("generates a new secret",
			func(ctx SpecContext, state bootstrapState) {
				secret, expires, err := resumeSecret(ctx, state, nil, ttl, now)
				Expect(err).NotTo(HaveOccurred())
				Expect(string(secret)).NotTo(Equal(generated))
				Expect(secret).To(HaveLen(26))
				Expect(expires).To(Equal(now.Add(ttl)))
			},
			Entry("without a state", bootstrapState{}),
			Entry("when the stored one expired", bootstrapState{State: stateBootstrapping, Secret: generated,
				SecretSHA256: secretHash([]byte(generated)), Expires: now}),
			Entry("when the stored one doesn't match its hash", bootstrapState{State: stateBootstrapping,
				Secret: generated, SecretSHA256: "changed", Expires: now.Add(time.Minute)}),
			Entry("after a hand-over", bootstrapState{State: stateHandedOver}),
		)
	})

	Context("newBootstrapperFromConfig", func() {
		It("stores a generated secret, and continues with it after a restart", func(ctx SpecContext) {
			var out bytes.Buffer
			b, err := newBootstrapperFromConfig(logged(ctx, &out), Config{}, dir, bootstrapState{})
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.Count(out.String(), string(b.secret))).To(Equal(1), "logged once")
			state, err := readBootstrapState(dir)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(bootstrapState{State: stateBootstrapping, Secret: string(b.secret),
				SecretSHA256: secretHash(b.secret), Expires: state.Expires}))
			Expect(state.Expires).To(BeTemporally("~", b.expires, time.Millisecond))

			out.Reset()
			again, err := newBootstrapperFromConfig(logged(ctx, &out), Config{}, dir, state)
			Expect(err).NotTo(HaveOccurred())
			Expect(again.secret).To(Equal(b.secret))
			Expect(again.expires).To(Equal(state.Expires))
			Expect(out.String()).NotTo(ContainSubstring(string(b.secret)), "not logged again")
		})

		It("doesn't store a secret that was set, only its hash", func(ctx SpecContext) {
			_, err := newBootstrapperFromConfig(ctx, Config{BootstrapSecret: testSecret}, dir, bootstrapState{})
			Expect(err).NotTo(HaveOccurred())
			data, err := os.ReadFile(stateFile(dir))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).NotTo(ContainSubstring(testSecret))
			Expect(string(data)).To(ContainSubstring(secretHash([]byte(testSecret))))
		})

		It("fails when it can't store the state", func(ctx SpecContext) {
			file := filepath.Join(dir, "file")
			Expect(os.WriteFile(file, nil, 0o600)).To(Succeed())
			_, err := newBootstrapperFromConfig(ctx, Config{BootstrapSecret: testSecret}, file, bootstrapState{})
			Expect(err).To(MatchError(ContainSubstring("failed to store the bootstrap state")))
		})
	})

	Context("checkStateInNormalMode", func() {
		It("removes the state of an unfinished bootstrap", func(ctx SpecContext) {
			state := bootstrapState{State: stateBootstrapping, Secret: "GENERATED"}
			Expect(writeBootstrapState(dir, state)).To(Succeed())
			Expect(checkStateInNormalMode(ctx, dir, state)).To(Succeed())
			Expect(exist(pki.APIBootstrapStateFile)).To(BeEmpty())
		})

		It("keeps the state of a hand-over and of a confirmed bootstrap", func(ctx SpecContext) {
			for _, s := range []string{stateHandedOver, stateConfirmed} {
				Expect(writeBootstrapState(dir, bootstrapState{State: s})).To(Succeed())
				Expect(checkStateInNormalMode(ctx, dir, bootstrapState{State: s})).To(Succeed())
				Expect(readBootstrapState(dir)).To(Equal(bootstrapState{State: s}))
			}
		})

		It("fails when it can't remove the state", func(ctx SpecContext) {
			Expect(os.MkdirAll(filepath.Join(stateFile(dir), "x"), 0o700)).To(Succeed())
			Expect(checkStateInNormalMode(ctx, dir, bootstrapState{State: stateBootstrapping})).
				To(MatchError(ContainSubstring("unfinished bootstrap")))
		})
	})

	Context("confirmBootstrap", func() {
		It("removes the archive and stores the confirmed state", func() {
			write(pki.APIBootstrapArchiveFile)
			Expect(writeBootstrapState(dir, bootstrapState{State: stateHandedOver})).To(Succeed())
			Expect(confirmBootstrap(dir)).To(Succeed())
			Expect(exist(pki.APIBootstrapArchiveFile)).To(BeEmpty())
			Expect(readBootstrapState(dir)).To(Equal(bootstrapState{State: stateConfirmed}))
		})

		It("creates no state without a bootstrap", func() {
			Expect(confirmBootstrap(dir)).To(Succeed())
			Expect(exist(pki.APIBootstrapStateFile)).To(BeEmpty())
		})
	})
})
