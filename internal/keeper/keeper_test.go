// Copyright 2026 PgVillage
// Copyright 2017 Sorint.lab
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

package keeper

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	pgmocks "github.com/pgvillage-tools/orion/internal/mock/postgresql"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
	"github.com/rs/zerolog"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
)

var _ = DescribeTable("parseSynchronousStandbyNames",
	func(in string, out []string, wantErr error) {
		got, err := parseSynchronousStandbyNames(in)
		if wantErr != nil {
			Expect(err).To(MatchError(wantErr.Error()))
			return
		}
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(out))
	},
	Entry(nil, "2 (orion_2c3870f3,orion_c874a3cb)", []string{"orion_2c3870f3", "orion_c874a3cb"}, nil),
	Entry(nil, "2 ( orion_2c3870f3 , orion_c874a3cb )", []string{"orion_2c3870f3", "orion_c874a3cb"}, nil),
	Entry(nil, "21 (\" orion_2c3870f3\",orion_c874a3cb)", []string{"\" orion_2c3870f3\"", "orion_c874a3cb"}, nil),
	Entry(nil, "orion_2c3870f3,orion_c874a3cb", []string{"orion_2c3870f3", "orion_c874a3cb"}, nil),
	Entry(nil, "node1", []string{"node1"}, nil),
	Entry(nil, "2 (node1,", []string{"node1"},
		errors.New("synchronous standby string has number but lacks brackets")),
)

var _ = Describe("generateHBA", func() {
	const (
		db1 = "db1"
		db2 = "db2"
		db3 = "db3"
	)
	const (
		defaultLine1 = "local postgres superuser md5"
		defaultLine2 = "local replication repluser md5"
	)

	// minimal clusterdata with only the fields used by generateHBA
	newClusterData := func() *cluster.Data {
		return &cluster.Data{
			Cluster: &cluster.Cluster{
				Spec:   &cluster.Spec{},
				Status: cluster.ClusterStatus{},
			},
			Keepers: cluster.Keepers{},
			DBs: cluster.DBs{
				db1: &cluster.DB{
					UID: db1,
					Spec: &cluster.DBSpec{
						Role: common.RolePrimary,
					},
					Status: cluster.DBStatus{
						ListenAddress: "192.168.0.1",
					},
				},
				db2: &cluster.DB{
					UID: db2,
					Spec: &cluster.DBSpec{
						Role: common.RoleReplica,
						FollowConfig: &cluster.FollowConfig{
							Type:  cluster.FollowTypeInternal,
							DBUID: db1,
						},
					},
					Status: cluster.DBStatus{
						ListenAddress: "192.168.0.2",
					},
				},
				db3: &cluster.DB{
					UID: db3,
					Spec: &cluster.DBSpec{
						Role: common.RoleReplica,
						FollowConfig: &cluster.FollowConfig{
							Type:  cluster.FollowTypeInternal,
							DBUID: db1,
						},
					},
					Status: cluster.DBStatus{
						ListenAddress: "192.168.0.3",
					},
				},
			},
			Proxy: &cluster.Proxy{},
		}
	}

	DescribeTable("generates the hba",
		func(mode cluster.SUReplAccessMode, dbUID string, pgHBA []string, out []string) {
			p := &PostgresKeeper{
				pgSUConnType:          "host",
				pgSULocalAuthMethod:   "md5",
				pgSUAuthMethod:        "md5",
				pgSUUsername:          "superuser",
				pgReplConnType:        "host",
				pgReplLocalAuthMethod: "md5",
				pgReplAuthMethod:      "md5",
				pgReplUsername:        "repluser",
			}
			cd := newClusterData()
			cd.Cluster.Spec.DefaultSUReplAccessMode = &mode
			db := cd.DBs[dbUID]
			db.Spec.PGHBA = pgHBA

			Expect(p.generateHBA(context.Background(), cd, db, false)).To(Equal(out))
		},
		Entry("all, primary", cluster.SUReplAccessAll, db1, nil, []string{
			defaultLine1,
			defaultLine2,
			"host all superuser 0.0.0.0/0 md5",
			"host all superuser ::0/0 md5",
			"host replication repluser 0.0.0.0/0 md5",
			"host replication repluser ::0/0 md5",
			"host all all 0.0.0.0/0 md5",
			"host all all ::0/0 md5",
		}),
		Entry("all, replica", cluster.SUReplAccessAll, db2, nil, []string{
			defaultLine1,
			defaultLine2,
			"host all superuser 0.0.0.0/0 md5",
			"host all superuser ::0/0 md5",
			"host replication repluser 0.0.0.0/0 md5",
			"host replication repluser ::0/0 md5",
			"host all all 0.0.0.0/0 md5",
			"host all all ::0/0 md5",
		}),
		Entry("all, primary, pghba", cluster.SUReplAccessAll, db1, []string{"host all all 192.168.0.0/24 md5"},
			[]string{
				defaultLine1,
				defaultLine2,
				"host all superuser 0.0.0.0/0 md5",
				"host all superuser ::0/0 md5",
				"host replication repluser 0.0.0.0/0 md5",
				"host replication repluser ::0/0 md5",
				"host all all 192.168.0.0/24 md5",
			}),
		Entry("all, replica, pghba", cluster.SUReplAccessAll, db2, []string{"host all all 192.168.0.0/24 md5"},
			[]string{
				defaultLine1,
				defaultLine2,
				"host all superuser 0.0.0.0/0 md5",
				"host all superuser ::0/0 md5",
				"host replication repluser 0.0.0.0/0 md5",
				"host replication repluser ::0/0 md5",
				"host all all 192.168.0.0/24 md5",
			}),
		Entry("strict, primary", cluster.SUReplAccessStrict, db1, nil, []string{
			defaultLine1,
			defaultLine2,
			"host all superuser 192.168.0.2/32 md5",
			"host replication repluser 192.168.0.2/32 md5",
			"host all superuser 192.168.0.3/32 md5",
			"host replication repluser 192.168.0.3/32 md5",
			"host all all 0.0.0.0/0 md5",
			"host all all ::0/0 md5",
		}),
		Entry("strict, replica", cluster.SUReplAccessStrict, db2, nil, []string{
			defaultLine1,
			defaultLine2,
			"host all all 0.0.0.0/0 md5",
			"host all all ::0/0 md5",
		}),
	)
})

var _ = Describe("getTimeLinesHistory", func() {
	var (
		ctx  context.Context
		ctrl *gomock.Controller
		pgm  *pgmocks.MockPGManager
	)

	BeforeEach(func() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(context.Background())
		DeferCleanup(cancel)
		ctrl = gomock.NewController(GinkgoT())
		DeferCleanup(ctrl.Finish)
		pgm = pgmocks.NewMockPGManager(ctrl)
	})

	It("should return empty if timelineID is not greater than 1", func() {
		pgState := &cluster.PostgresState{
			TimelineID: 1,
		}

		ctlsh, err := getTimeLinesHistory(ctx, pgState, nil, 3)

		Expect(err).NotTo(HaveOccurred())
		Expect(ctlsh).To(BeEmpty())
	})

	It("should return error if there is error in getting timeline history", func() {
		var timelineID uint64 = 2
		pgState := &cluster.PostgresState{
			TimelineID: timelineID,
		}

		pgm.EXPECT().GetTimelinesHistory(timelineID).Return(
			[]*pg.TimelineHistory{},
			errors.New("failed to get timeline history"))
		ctlsh, err := getTimeLinesHistory(ctx, pgState, pgm, 3)

		Expect(err).To(MatchError("failed to get timeline history"))
		Expect(ctlsh).To(BeEmpty())
	})

	It("should return timeline history as is if the given length is less than maxPostgresTimelinesHistory", func() {
		var timelineID uint64 = 2
		pgState := &cluster.PostgresState{
			TimelineID: timelineID,
		}

		timelineHistories := []*pg.TimelineHistory{
			{
				TimelineID:  1,
				SwitchPoint: 1,
				Reason:      "reason1",
			},
			{
				TimelineID:  2,
				SwitchPoint: 2,
				Reason:      "reason2",
			},
		}
		pgm.EXPECT().GetTimelinesHistory(timelineID).Return(timelineHistories, nil)
		ctlsh, err := getTimeLinesHistory(ctx, pgState, pgm, 3)

		Expect(err).NotTo(HaveOccurred())
		Expect(ctlsh).To(Equal(cluster.PostgresTimelinesHistory{
			&cluster.PostgresTimelineHistory{
				TimelineID:  1,
				SwitchPoint: 1,
				Reason:      "reason1",
			},
			&cluster.PostgresTimelineHistory{
				TimelineID:  2,
				SwitchPoint: 2,
				Reason:      "reason2",
			},
		}))
	})

	It("should return timeline history with last maxPostgresTimelinesHistory elements if timeline history "+
		"length is greater than maxPostgresTimelinesHistory", func() {
		var timelineID uint64 = 4
		pgState := &cluster.PostgresState{
			TimelineID: timelineID,
		}

		timelineHistories := []*pg.TimelineHistory{
			{
				TimelineID:  1,
				SwitchPoint: 1,
				Reason:      "reason1",
			},
			{
				TimelineID:  2,
				SwitchPoint: 2,
				Reason:      "reason2",
			},
			{
				TimelineID:  3,
				SwitchPoint: 3,
				Reason:      "reason3",
			},
			{
				TimelineID:  4,
				SwitchPoint: 4,
				Reason:      "reason4",
			},
		}
		pgm.EXPECT().GetTimelinesHistory(timelineID).Return(timelineHistories, nil)
		ctlsh, err := getTimeLinesHistory(ctx, pgState, pgm, 2)

		Expect(err).NotTo(HaveOccurred())
		Expect(ctlsh).To(Equal(cluster.PostgresTimelinesHistory{
			&cluster.PostgresTimelineHistory{
				TimelineID:  3,
				SwitchPoint: 3,
				Reason:      "reason3",
			},
			&cluster.PostgresTimelineHistory{
				TimelineID:  4,
				SwitchPoint: 4,
				Reason:      "reason4",
			},
		}))
	})
})

// the hba used while a synchronous primary waits for its sync standbys (onlyInternal=true): only the superuser and
// replication user entries are generated, so normal users are locked out, and the spec PGHBA entries are left out.
var _ = Describe("generateHBA with only internal users", func() {
	// hbaTestKeeper returns a keeper with the user and auth settings used by the hba tests.
	hbaTestKeeper := func() *PostgresKeeper {
		return &PostgresKeeper{
			pgSUConnType:          connTypeHost,
			pgSULocalAuthMethod:   authMd5,
			pgSUAuthMethod:        authMd5,
			pgSUUsername:          "superuser",
			pgReplConnType:        connTypeHost,
			pgReplLocalAuthMethod: authMd5,
			pgReplAuthMethod:      authMd5,
			pgReplUsername:        "repluser",
		}
	}

	// hbaTestClusterData returns a cluster with a primary (db1) and two replicas (db2, db3).
	hbaTestClusterData := func(mode cluster.SUReplAccessMode) *cluster.Data {
		replica := func(uid, address string) *cluster.DB {
			return &cluster.DB{
				UID: uid,
				Spec: &cluster.DBSpec{
					Role:         common.RoleReplica,
					FollowConfig: &cluster.FollowConfig{Type: cluster.FollowTypeInternal, DBUID: "db1"},
				},
				Status: cluster.DBStatus{ListenAddress: address},
			}
		}
		return &cluster.Data{
			Cluster: &cluster.Cluster{Spec: &cluster.Spec{DefaultSUReplAccessMode: &mode}},
			DBs: cluster.DBs{
				"db1": &cluster.DB{
					UID:    "db1",
					Spec:   &cluster.DBSpec{Role: common.RolePrimary},
					Status: cluster.DBStatus{ListenAddress: "192.168.0.1"},
				},
				"db2": replica("db2", "192.168.0.2"),
				"db3": replica("db3", "192.168.0.3"),
			},
		}
	}

	const (
		local1 = "local postgres superuser md5"
		local2 = "local replication repluser md5"
	)
	allEntries := []string{
		local1,
		local2,
		"host all superuser 0.0.0.0/0 md5",
		"host all superuser ::0/0 md5",
		"host replication repluser 0.0.0.0/0 md5",
		"host replication repluser ::0/0 md5",
	}
	strictPrimaryEntries := []string{
		local1,
		local2,
		"host all superuser 192.168.0.2/32 md5",
		"host replication repluser 192.168.0.2/32 md5",
		"host all superuser 192.168.0.3/32 md5",
		"host replication repluser 192.168.0.3/32 md5",
	}
	customHBA := []string{"host all all 192.168.0.0/24 md5", "hostssl app appuser 10.0.0.0/8 scram-sha-256"}

	DescribeTable("locks normal users out",
		func(mode cluster.SUReplAccessMode, dbUID string, pgHBA []string, want []string) {
			ctx := context.Background()
			cd := hbaTestClusterData(mode)
			db := cd.DBs[dbUID]
			db.Spec.PGHBA = pgHBA
			out := hbaTestKeeper().generateHBA(ctx, cd, db, true)
			Expect(out).To(Equal(want))
			for _, e := range out {
				Expect(e).NotTo(ContainSubstring(" all all "), "onlyInternal hba lets normal users in")
				Expect(e).NotTo(ContainSubstring("appuser"), "onlyInternal hba lets normal users in")
			}
			// the same input without the lockout opens up to normal users again
			open := hbaTestKeeper().generateHBA(ctx, cd, db, false)
			Expect(len(open)).To(BeNumerically(">", len(out)),
				"hba without onlyInternal should have more entries than %v, got %v", out, open)
		},
		Entry("all, primary", cluster.SUReplAccessAll, "db1", nil, allEntries),
		Entry("all, replica", cluster.SUReplAccessAll, "db2", nil, allEntries),
		Entry("all, primary, custom pghba", cluster.SUReplAccessAll, "db1", customHBA, allEntries),
		Entry("strict, primary", cluster.SUReplAccessStrict, "db1", nil, strictPrimaryEntries),
		Entry("strict, primary, custom pghba", cluster.SUReplAccessStrict, "db1", customHBA, strictPrimaryEntries),
		Entry("strict, replica", cluster.SUReplAccessStrict, "db2", nil, []string{local1, local2}),
		Entry("strict, replica, custom pghba", cluster.SUReplAccessStrict, "db2", customHBA,
			[]string{local1, local2}),
	)

	// cert, which postgres doesn't support for local connections, becomes peer in the local entries, while the host
	// entries keep cert.
	It("uses peer instead of cert for the local entries", func() {
		p := hbaTestKeeper()
		p.pgSUAuthMethod = authCert
		p.pgSULocalAuthMethod = authCert
		p.pgReplAuthMethod = authCert
		p.pgReplLocalAuthMethod = authCert
		cd := hbaTestClusterData(cluster.SUReplAccessAll)
		Expect(p.generateHBA(context.Background(), cd, cd.DBs["db1"], true)).To(Equal([]string{
			"local postgres superuser peer",
			"local replication repluser peer",
			"host all superuser 0.0.0.0/0 cert",
			"host all superuser ::0/0 cert",
			"host replication repluser 0.0.0.0/0 cert",
			"host replication repluser ::0/0 cert",
		}))
	})
})

var _ = DescribeTable("localAuthMethod",
	func(in, want string) {
		Expect(localAuthMethod(context.Background(), in)).To(Equal(want))
	},
	Entry(nil, authCert, authPeer),
	Entry(nil, authMd5, authMd5),
	Entry(nil, authTrust, authTrust),
	Entry(nil, authPeer, authPeer),
	Entry(nil, authIdent, authIdent),
)

func writeFile(t testing.TB, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	// WriteFile applies the umask and keeps the mode of an existing file; set it explicitly
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

var _ = Describe("readPasswordFromFile", func() {
	// keeperLogCtx returns a context whose keeper logger writes to buf, so a test can check the warnings.
	keeperLogCtx := func(buf *bytes.Buffer) context.Context {
		_, def := logging.GetLogComponent(context.Background(), logging.KeeperComponent)
		l := zerolog.New(buf).Level(def.GetLevel())
		return l.WithContext(context.Background())
	}

	const (
		tooOpen  = "password file permissions are too open"
		password = "secret"
	)
	var dir string

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
	})

	// The trailing newline is stripped later, by validateUsersConfig; see the ValidateUsersConfig specs.
	DescribeTable("returns the content as is, trailing newline included",
		func(content string) {
			path := filepath.Join(dir, "pw")
			writeFile(GinkgoTB(), path, content, ownerRWPermisions)
			var buf bytes.Buffer
			got, err := readPasswordFromFile(keeperLogCtx(&buf), path)
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(content))
			Expect(buf.String()).NotTo(ContainSubstring(tooOpen), "unexpected warning for mode 0600")
		},
		Entry(nil, password),
		Entry(nil, "secret\n"),
		Entry(nil, "secret\r\n"),
		Entry(nil, "  sp ace\tx \n\n"),
		Entry(nil, ""),
	)

	It("fails for a missing file, naming it", func() {
		path := filepath.Join(dir, "missing")
		_, err := readPasswordFromFile(context.Background(), path)
		Expect(err).To(MatchError(ContainSubstring(path)))
	})

	It("fails for a directory", func() {
		_, err := readPasswordFromFile(context.Background(), dir)
		Expect(err).To(HaveOccurred(), "expected an error reading a directory")
	})

	DescribeTable("modes",
		func(mode os.FileMode, warn bool) {
			path := filepath.Join(dir, fmt.Sprintf("pw-%o", mode))
			writeFile(GinkgoTB(), path, password, mode)
			var buf bytes.Buffer
			got, err := readPasswordFromFile(keeperLogCtx(&buf), path)
			// a too open file is only warned about, it is still read
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(password))
			Expect(strings.Contains(buf.String(), tooOpen)).To(Equal(warn), "log: %s", buf.String())
		},
		Entry(nil, os.FileMode(0o400), false),
		Entry(nil, os.FileMode(ownerRWPermisions), false),
		// only the owner: fine, whatever the owner bits
		Entry(nil, os.FileMode(ownerRWExecPermisions), false),
		// any bit for the group or others is too open
		Entry(nil, os.FileMode(0o640), true),
		Entry(nil, os.FileMode(ownerRWOtherRPermisions), true),
		Entry(nil, os.FileMode(0o666), true),
		Entry(nil, os.FileMode(0o466), true),
		Entry(nil, os.FileMode(0o404), true),
		Entry(nil, os.FileMode(0o444), true),
	)

	It("follows symlinks", func() {
		// Kubernetes mounts secrets as symlinks: the mode of the target counts, not the one of the link.
		target := filepath.Join(dir, "target")
		writeFile(GinkgoTB(), target, password, ownerRWPermisions)
		link := filepath.Join(dir, "link")
		Expect(os.Symlink(target, link)).To(Succeed())
		var buf bytes.Buffer
		got, err := readPasswordFromFile(keeperLogCtx(&buf), link)
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(password))
		Expect(buf.String()).NotTo(ContainSubstring(tooOpen), "unexpected warning for a symlink to a 0600 file")
		openTarget := filepath.Join(dir, "open-target")
		writeFile(GinkgoTB(), openTarget, password, ownerRWOtherRPermisions)
		openLink := filepath.Join(dir, "open-link")
		Expect(os.Symlink(openTarget, openLink)).To(Succeed())
		buf.Reset()
		_, err = readPasswordFromFile(keeperLogCtx(&buf), openLink)
		Expect(err).NotTo(HaveOccurred())
		Expect(buf.String()).To(ContainSubstring(tooOpen), "expected a warning for a symlink to a 0644 file")
	})
})

// the user, auth method and password rules of the keeper (stolon TestInitUsers)
var _ = Describe("ValidateUsersConfig", func() {
	var dir, suFile, suNLFile, replFile, onlyNLFile string

	pwFile := func(name, content string) string {
		path := filepath.Join(dir, name)
		writeFile(GinkgoTB(), path, content, ownerRWPermisions)
		return path
	}
	// valid returns the settings of the flag defaults, with passwords
	valid := func() *Config {
		return &Config{
			PGSUUsername: "postgres", PGSUAuthMethod: authMd5, PGSUConnType: connTypeHost, PGSUPassword: "supass",
			PGReplUsername: "repl", PGReplAuthMethod: authMd5, PGReplConnType: connTypeHost, PGReplPassword: "replpass",
		}
	}
	sameUser := func(c *Config) { c.PGSUUsername, c.PGReplUsername = "pg", "pg" }

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		suFile = pwFile("su", "supass")
		suNLFile = pwFile("su-nl", "supass\n")
		replFile = pwFile("repl", "replpass")
		onlyNLFile = pwFile("only-nl", "\n")
	})

	DescribeTable("validates",
		// want is the error, or "" when the settings are valid
		func(change func(c *Config), want string) {
			c := valid()
			change(c)
			err := ValidateUsersConfig(context.Background(), c)
			if want == "" {
				Expect(err).NotTo(HaveOccurred())
				return
			}
			Expect(err).To(MatchError(ContainSubstring(want)))
		},
		Entry("valid with passwords", func(*Config) {}, ""),
		Entry("valid with password files", func(c *Config) {
			c.PGSUPassword, c.PGSUPasswordFile, c.PGReplPassword, c.PGReplPasswordFile = "", suFile, "", replFile
		}, ""),
		Entry("repl username required", func(c *Config) { c.PGReplUsername = "" }, "--pg-repl-username is required"),
		Entry("repl password required", func(c *Config) { c.PGReplPassword = "" },
			"one of --pg-repl-password or --pg-repl-passwordfile is required"),
		Entry("only one repl password", func(c *Config) { c.PGReplPasswordFile = replFile },
			"only one of --pg-repl-password or --pg-repl-passwordfile must be provided"),
		Entry("su password required", func(c *Config) { c.PGSUPassword = "" },
			"one of --pg-su-password or --pg-su-passwordfile is required"),
		Entry("only one su password", func(c *Config) { c.PGSUPasswordFile = suFile },
			"only one of --pg-su-password or --pg-su-passwordfile must be provided"),
		Entry("missing password file", func(c *Config) {
			c.PGSUPassword, c.PGSUPasswordFile = "", filepath.Join(dir, "nope")
		}, "cannot read pg superuser password"),
		Entry("same user, same password", func(c *Config) {
			sameUser(c)
			c.PGSUPassword, c.PGReplPassword = "pass", "pass"
		}, ""),
		Entry("same user, different password", func(c *Config) {
			sameUser(c)
			c.PGSUPassword, c.PGReplPassword = "pass1", "pass2"
		}, "provided superuser name and replication user name are the same but provided passwords are different"),
		Entry("same user, password file has a trailing newline", func(c *Config) {
			sameUser(c)
			c.PGSUPassword, c.PGSUPasswordFile, c.PGReplPassword = "", suNLFile, "supass"
		}, ""),
		Entry("same user, different auth methods", func(c *Config) {
			sameUser(c)
			c.PGSUAuthMethod, c.PGSUPassword = authCert, ""
		}, "do not support different auth methods when utilizing superuser for replication"),
		Entry("password file with only a newline", func(c *Config) {
			c.PGSUPassword, c.PGSUPasswordFile = "", onlyNLFile
		}, "superuser password is empty after removing trailing new line"),
		Entry("repl trust with password", func(c *Config) { c.PGReplAuthMethod = authTrust },
			"can not utilize --pg-repl-auth-method trust together with --pg-repl-password"),
		Entry("su trust with password", func(c *Config) { c.PGSUAuthMethod = authTrust },
			"can not utilize --pg-su-auth-method trust"),
		Entry("su local trust with password", func(c *Config) { c.PGSULocalAuthMethod = authTrust },
			"can not utilize --pg-su-auth-method trust"),
		Entry("trust without passwords", func(c *Config) {
			c.PGSUAuthMethod, c.PGSUPassword, c.PGReplAuthMethod, c.PGReplPassword = authTrust, "", authTrust, ""
		}, ""),
		Entry("cert without passwords", func(c *Config) {
			c.PGSUAuthMethod, c.PGSUPassword, c.PGReplAuthMethod, c.PGReplPassword = authCert, "", authCert, ""
		}, ""),
		Entry("peer is a valid auth method", func(c *Config) { c.PGSULocalAuthMethod = authPeer }, ""),
		Entry("invalid repl auth method", func(c *Config) { c.PGReplAuthMethod = "bogus" },
			"--pg-repl-auth-method must be one of: md5, trust, cert, ident or peer"),
		Entry("invalid su auth method", func(c *Config) { c.PGSUAuthMethod = "bogus" },
			"--pg-su-auth-method must be one of: md5, trust, cert, ident or peer"),
		Entry("invalid su connection type", func(c *Config) { c.PGSUConnType = "tcp" },
			"--pg-su-connection-type must be one of"),
	)

	It("fills the local auth methods and trims the passwords", func() {
		c := valid()
		c.PGSUPassword, c.PGSUPasswordFile = "", suNLFile
		Expect(ValidateUsersConfig(context.Background(), c)).To(Succeed())
		Expect(c.PGSULocalAuthMethod).To(Equal(authMd5))
		Expect(c.PGReplLocalAuthMethod).To(Equal(authMd5))
		Expect(c.PGSUPassword).To(Equal("supass"))
	})
})

var _ = Describe("the local state", func() {
	It("DBLocalState.DeepCopy returns an independent copy", func() {
		var nilState *DBLocalState
		Expect(nilState.DeepCopy()).To(BeNil(), "DeepCopy of nil should be nil")
		s := &DBLocalState{
			UID:              "uid",
			Generation:       3,
			Initializing:     true,
			InitPGParameters: common.Parameters{"shared_buffers": "128MB"},
		}
		c := s.DeepCopy()
		Expect(c).NotTo(BeIdenticalTo(s))
		Expect(c).To(Equal(s))
		c.InitPGParameters["shared_buffers"] = "1GB"
		Expect(s.InitPGParameters).To(HaveKeyWithValue("shared_buffers", "128MB"),
			"changing the copy changed the original")
		p := &PostgresKeeper{dbLocalState: s}
		dc := p.dbLocalStateCopy()
		Expect(dc).NotTo(BeIdenticalTo(s))
		Expect(dc).To(Equal(s))
	})

	It("is saved in and loaded from files", func() {
		p := &PostgresKeeper{cfg: &Config{DataDir: GinkgoT().TempDir()}}

		Expect(os.IsNotExist(p.loadKeeperLocalState())).To(BeTrue(), "expected a not exist error")
		Expect(os.IsNotExist(p.loadDBLocalState())).To(BeTrue(), "expected a not exist error")

		kls := &LocalState{UID: "keeper1", ClusterUID: "cluster1"}
		p.keeperLocalState = kls
		Expect(p.saveKeeperLocalState()).To(Succeed())
		p.keeperLocalState = nil
		Expect(p.loadKeeperLocalState()).To(Succeed())
		Expect(p.keeperLocalState).To(Equal(kls))

		dbls := &DBLocalState{UID: "db1", Generation: 2, InitPGParameters: common.Parameters{"a": "b"}}
		Expect(p.saveDBLocalState(dbls)).To(Succeed())
		Expect(p.dbLocalState).NotTo(BeIdenticalTo(dbls), "saveDBLocalState should keep a copy in memory")
		Expect(p.dbLocalState).To(Equal(dbls), "saveDBLocalState should keep a copy in memory")
		p.dbLocalState = nil
		Expect(p.loadDBLocalState()).To(Succeed())
		Expect(p.dbLocalState).To(Equal(dbls))

		for _, path := range []string{p.keeperLocalStateFilePath(), p.dbLocalStateFilePath()} {
			fi, err := os.Stat(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(fi.Mode().Perm()).To(Equal(os.FileMode(ownerRWPermisions)), "%s", path)
			writeFile(GinkgoTB(), path, "{not json", ownerRWPermisions)
		}
		Expect(p.loadKeeperLocalState()).NotTo(Succeed(), "expected an error loading a corrupt keeper state")
		Expect(p.loadDBLocalState()).NotTo(Succeed(), "expected an error loading a corrupt db state")
	})
})

var _ = Describe("IsMaster", func() {
	db := func(role common.Role, ft cluster.FollowType) *cluster.DB {
		return &cluster.DB{Spec: &cluster.DBSpec{Role: role, FollowConfig: &cluster.FollowConfig{Type: ft}}}
	}

	It("is true for a primary", func() {
		Expect(IsMaster(db(common.RolePrimary, ""))).To(BeTrue())
	})
	It("is false for an internal replica", func() {
		Expect(IsMaster(db(common.RoleReplica, cluster.FollowTypeInternal))).To(BeFalse())
	})
	It("is true for the replica of an external primary (standby cluster)", func() {
		Expect(IsMaster(db(common.RoleReplica, cluster.FollowTypeExternal))).To(BeTrue())
	})
	It("panics for an undefined role", func() {
		Expect(func() { IsMaster(db(common.RoleUndefined, "")) }).To(Panic())
	})
})

var _ = DescribeTable("usePgrewind",
	func(user, password, auth string, dbUsePgrewind, want bool) {
		p := &PostgresKeeper{pgSUUsername: user, pgSUPassword: password, pgSUAuthMethod: auth}
		db := &cluster.DB{Spec: &cluster.DBSpec{UsePgrewind: dbUsePgrewind}}
		Expect(p.usePgrewind(db)).To(Equal(want))
	},
	Entry(nil, "su", "pw", authMd5, true, true),
	Entry(nil, "su", "", authCert, true, true),
	Entry(nil, "su", "pw", authMd5, false, false),
	Entry(nil, "su", "", authMd5, true, false),
	Entry(nil, "", "pw", authMd5, true, false),
)

var _ = DescribeTable("walKeepSegments and walKeepSize",
	func(params cluster.PGParameters, segments int, size string) {
		p := &PostgresKeeper{}
		db := &cluster.DB{Spec: &cluster.DBSpec{PGParameters: params}}
		Expect(p.walKeepSegments(db)).To(Equal(segments))
		Expect(p.walKeepSize(db)).To(Equal(size))
	},
	Entry(nil, nil, minWalKeepSegments, "128"),
	Entry(nil, cluster.PGParameters{"wal_keep_segments": "100", "wal_keep_size": "2GB"}, 100, "2GB"),
	// lower than the minimum or invalid segments are ignored; wal_keep_size is returned as is
	Entry(nil, cluster.PGParameters{"wal_keep_segments": "2", "wal_keep_size": "1"}, minWalKeepSegments, "1"),
	Entry(nil, cluster.PGParameters{"wal_keep_segments": "lots"}, minWalKeepSegments, "128"),
)

// fakePGManager returns a pg.Manager whose postgres binary reports the given version.
// With an empty version there is no postgres binary.
func fakePGManager(t testing.TB, version string) *pg.Manager {
	t.Helper()
	dir := t.TempDir()
	if version != "" {
		script := fmt.Sprintf("#!/bin/sh\necho 'postgres (PostgreSQL) %s'\n", version)
		writeFile(t, filepath.Join(dir, "postgres"), script, ownerRWExecPermisions)
	}
	return pg.NewManager(dir, dir, "", nil, nil, "", "", "", "", "", "", 0)
}

var _ = DescribeTable("walLevel and mandatoryPGParameters",
	func(version string, params cluster.PGParameters, walLevel, walKey, walValue string) {
		ctx := context.Background()
		p := &PostgresKeeper{pgm: fakePGManager(GinkgoTB(), version)}
		db := &cluster.DB{Spec: &cluster.DBSpec{PGParameters: params}}
		Expect(p.walLevel(ctx, db)).To(Equal(walLevel))
		want := common.Parameters{
			"unix_socket_directories": common.PgUnixSocketDirectories,
			"wal_level":               walLevel,
			"hot_standby":             "on",
		}
		if walKey != "" {
			want[walKey] = walValue
		}
		Expect(p.mandatoryPGParameters(ctx, db)).To(Equal(want))
	},
	Entry("no binary", "", nil, "hot_standby", "", ""),
	Entry("9.5", "9.5.25", nil, "hot_standby", "wal_keep_segments", "8"),
	Entry("12", "12.3", nil, "replica", "wal_keep_segments", "8"),
	Entry("12 logical", "12.3", cluster.PGParameters{"wal_level": "logical"}, "logical", "wal_keep_segments", "8"),
	Entry("16", "16.2", nil, "replica", "wal_keep_size", "128"),
	Entry("16 minimal is ignored", "16.2", cluster.PGParameters{"wal_level": "minimal"}, "replica", "wal_keep_size",
		"128"),
)

var _ = Describe("createPGParameters", func() {
	var (
		ctx context.Context
		p   *PostgresKeeper
	)
	spec := func() *cluster.DBSpec {
		return &cluster.DBSpec{
			MaxStandbys:          5,
			AdditionalWalSenders: 4,
			PGParameters:         cluster.PGParameters{"work_mem": "8MB", "hot_standby": "off"},
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		p = &PostgresKeeper{
			pgm:             fakePGManager(GinkgoTB(), "16.2"),
			pgListenAddress: "10.0.0.1",
			pgPort:          "5433",
			dbLocalState:    &DBLocalState{InitPGParameters: common.Parameters{"init_only": "x", "port": "1"}},
		}
	})

	It("overrides the user parameters with the managed ones", func() {
		Expect(p.createPGParameters(ctx, &cluster.DB{Spec: spec()})).To(Equal(common.Parameters{
			"work_mem":                  "8MB",
			"hot_standby":               "on",
			"unix_socket_directories":   common.PgUnixSocketDirectories,
			"wal_level":                 "replica",
			"wal_keep_size":             "128",
			"listen_addresses":          "10.0.0.1",
			"port":                      "5433",
			"max_replication_slots":     "5",
			"max_wal_senders":           "16", // 5*2 + 2 + 4
			"synchronous_standby_names": "",
		}))
	})

	It("includes the config and sets wal_log_hints for pg_rewind", func() {
		s := spec()
		s.IncludeConfig = true
		s.UsePgrewind = true
		got := p.createPGParameters(ctx, &cluster.DB{Spec: s})
		Expect(got).To(HaveKeyWithValue("init_only", "x"))
		Expect(got).To(HaveKeyWithValue("port", "5433"))
		Expect(got).To(HaveKeyWithValue("wal_log_hints", "on"))
	})

	DescribeTable("sets synchronous_standby_names",
		func(syncRepl bool, sync, external []string, want string) {
			s := spec()
			s.SynchronousReplication = syncRepl
			s.SynchronousStandbys = sync
			s.ExternalSynchronousStandbys = external
			got := p.createPGParameters(ctx, &cluster.DB{Spec: s})["synchronous_standby_names"]
			Expect(got).To(Equal(want))
			// what the keeper writes must parse back to the same standbys
			if got != "" {
				names, err := parseSynchronousStandbyNames(got)
				Expect(err).NotTo(HaveOccurred())
				Expect(names).To(HaveLen(len(sync) + len(external)))
			}
		},
		Entry("synchronous, no standbys", true, nil, nil, ""),
		Entry("standbys, not synchronous", false, []string{"a"}, nil, ""),
		Entry("one standby", true, []string{"a"}, nil, common.OrionName("a")),
		Entry("one external standby", true, nil, []string{"ext"}, "ext"),
		Entry("standbys and an external standby", true, []string{"a", "b"}, []string{"ext"},
			fmt.Sprintf("3 (%s,%s,ext)", common.OrionName("a"), common.OrionName("b"))),
	)
})

var _ = Describe("createRecoveryOptions", func() {
	p := &PostgresKeeper{}

	It("standby, no target", func() {
		got := p.createRecoveryOptions(pg.RecoveryModeStandby,
			&cluster.StandbySettings{PrimaryConninfo: "host=x", PrimarySlotName: "slot", RecoveryMinApplyDelay: "5min"},
			&cluster.ArchiveRecoverySettings{RestoreCommand: "cp %f %p"},
			nil)
		Expect(got).To(Equal(&pg.RecoveryOptions{
			RecoveryMode: pg.RecoveryModeStandby,
			RecoveryParameters: common.Parameters{
				"primary_conninfo":         "host=x",
				"primary_slot_name":        "slot",
				"recovery_min_apply_delay": "5min",
				"restore_command":          "cp %f %p",
				"recovery_target_timeline": "latest",
			},
		}))
	})
	It("leaves empty settings out", func() {
		got := p.createRecoveryOptions(pg.RecoveryModeStandby, &cluster.StandbySettings{}, nil, nil)
		Expect(got.RecoveryParameters).To(Equal(common.Parameters{"recovery_target_timeline": "latest"}))
	})
	It("pitr target", func() {
		got := p.createRecoveryOptions(pg.RecoveryModeRecovery, nil, nil, &cluster.RecoveryTargetSettings{
			RecoveryTarget:         "immediate",
			RecoveryTargetLsn:      "0/3000000",
			RecoveryTargetName:     "before",
			RecoveryTargetTime:     "2026-01-01 00:00:00",
			RecoveryTargetXid:      "1234",
			RecoveryTargetTimeline: "2",
		})
		Expect(got.RecoveryMode).To(Equal(pg.RecoveryModeRecovery))
		Expect(got.RecoveryParameters).To(Equal(common.Parameters{
			"recovery_target":          "immediate",
			"recovery_target_lsn":      "0/3000000",
			"recovery_target_name":     "before",
			"recovery_target_time":     "2026-01-01 00:00:00",
			"recovery_target_xid":      "1234",
			"recovery_target_timeline": "2",
			"recovery_target_action":   "promote",
		}))
	})
	It("empty pitr target has no timeline", func() {
		got := p.createRecoveryOptions(pg.RecoveryModeRecovery, nil, nil, &cluster.RecoveryTargetSettings{})
		Expect(got.RecoveryParameters).To(Equal(common.Parameters{"recovery_target_action": "promote"}))
	})
})

var _ = Describe("the connection parameters", func() {
	db := &cluster.DB{UID: "db2"}
	followed := &cluster.DB{UID: "db1", Status: cluster.DBStatus{ListenAddress: "10.0.0.1", Port: "5432"}}
	newKeeper := func(auth string) *PostgresKeeper {
		return &PostgresKeeper{
			pgSUUsername: "su", pgSUPassword: "supw", pgSUAuthMethod: auth,
			pgReplUsername: "repl", pgReplPassword: "replpw", pgReplAuthMethod: auth,
			pgReplSslMode: "require", pgPort: "5433",
		}
	}
	appName := common.OrionName(db.UID)

	DescribeTable("have the password only with md5",
		func(get func(p *PostgresKeeper) pg.ConnParams, want pg.ConnParams) {
			// the password is only part of the conn params with md5
			got := get(newKeeper(authCert))
			Expect(got.Equals(want)).To(BeTrue(), "cert: got %v, want %v", got, want)
			wantMd5 := want.Copy()
			if want[pg.ConnParamKeyUser] == "su" {
				wantMd5[pg.ConnParamKeyPassword] = "supw"
			} else {
				wantMd5[pg.ConnParamKeyPassword] = "replpw"
			}
			got = get(newKeeper(authMd5))
			Expect(got.Equals(wantMd5)).To(BeTrue(), "md5: got %v, want %v", got, wantMd5)
		},
		Entry("su", func(p *PostgresKeeper) pg.ConnParams { return p.getSUConnParams(db, followed) }, pg.ConnParams{
			pg.ConnParamKeyUser: "su", pg.ConnParamKeyHost: "10.0.0.1", pg.ConnParamKeyPort: "5432",
			pg.ConnParamKeyAppName: appName, pg.ConnParamKeyDbName: defaultDatabase,
			pg.ConnParamKeySSLMode: "require",
		}),
		Entry("repl", func(p *PostgresKeeper) pg.ConnParams { return p.getReplConnParams(db, followed) },
			pg.ConnParams{
				pg.ConnParamKeyUser: "repl", pg.ConnParamKeyHost: "10.0.0.1", pg.ConnParamKeyPort: "5432",
				pg.ConnParamKeyAppName: appName, pg.ConnParamKeySSLMode: "prefer",
			}),
		Entry("local", func(p *PostgresKeeper) pg.ConnParams { return p.getLocalConnParams() }, pg.ConnParams{
			pg.ConnParamKeyUser: "su", pg.ConnParamKeyHost: common.PgUnixSocketDirectories,
			pg.ConnParamKeyPort: "5433", pg.ConnParamKeyDbName: defaultDatabase,
		}),
		Entry("local repl", func(p *PostgresKeeper) pg.ConnParams { return p.getLocalReplConnParams() },
			pg.ConnParams{
				pg.ConnParamKeyUser: "repl", pg.ConnParamKeyHost: common.PgUnixSocketDirectories,
				pg.ConnParamKeyPort: "5433",
			}),
	)
})

var _ = Describe("isDifferentTimelineBranch", func() {
	history := func(switchPoints ...uint64) cluster.PostgresTimelinesHistory {
		h := cluster.PostgresTimelinesHistory{}
		for i, sp := range switchPoints {
			h = append(h, &cluster.PostgresTimelineHistory{TimelineID: uint64(i + 1), SwitchPoint: sp})
		}
		return h
	}

	DescribeTable("tells if we are on another timeline branch than the followed db",
		func(followedTL uint64, followedHist cluster.PostgresTimelinesHistory, ourTL, ourPos uint64,
			ourHist cluster.PostgresTimelinesHistory, want bool) {
			p := &PostgresKeeper{}
			followed := &cluster.DB{Status: cluster.DBStatus{TimelineID: followedTL, TimelinesHistory: followedHist}}
			state := &cluster.PostgresState{TimelineID: ourTL, XLogPos: ourPos, TimelinesHistory: ourHist}
			Expect(p.isDifferentTimelineBranch(context.Background(), followed, state)).To(Equal(want))
		},
		Entry("followed is behind us", uint64(1), nil, uint64(2), uint64(0), nil, true),
		Entry("same timeline 1", uint64(1), nil, uint64(1), uint64(0), nil, false),
		Entry("same timeline, no history", uint64(2), nil, uint64(2), uint64(0), nil, false),
		Entry("same timeline, same switch point", uint64(2), history(100), uint64(2), uint64(0), history(100),
			false),
		Entry("same timeline, different switch point", uint64(2), history(100), uint64(2), uint64(0), history(200),
			true),
		Entry("followed ahead, forked after our position", uint64(2), history(100), uint64(1), uint64(50), nil,
			false),
		Entry("followed ahead, forked at our position", uint64(2), history(100), uint64(1), uint64(100), nil,
			false),
		Entry("followed ahead, forked before our position", uint64(2), history(100), uint64(1), uint64(150), nil,
			true),
		Entry("followed ahead, no history", uint64(2), nil, uint64(1), uint64(150), nil, false),
	)
})
