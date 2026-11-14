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

package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	pgmocks "github.com/pgvillage-tools/orion/internal/mock/postgresql"
	pg "github.com/pgvillage-tools/orion/internal/postgresql"
	"github.com/rs/zerolog"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/logging"
)

func TestParseSynchronousStandbyNames(t *testing.T) {
	tests := []struct {
		in  string
		out []string
		err error
	}{
		{
			in:  "2 (orion_2c3870f3,orion_c874a3cb)",
			out: []string{"orion_2c3870f3", "orion_c874a3cb"},
		},
		{
			in:  "2 ( orion_2c3870f3 , orion_c874a3cb )",
			out: []string{"orion_2c3870f3", "orion_c874a3cb"},
		},
		{
			in:  "21 (\" orion_2c3870f3\",orion_c874a3cb)",
			out: []string{"\" orion_2c3870f3\"", "orion_c874a3cb"},
		},
		{
			in:  "orion_2c3870f3,orion_c874a3cb",
			out: []string{"orion_2c3870f3", "orion_c874a3cb"},
		},
		{
			in:  "node1",
			out: []string{"node1"},
		},
		{
			in:  "2 (node1,",
			out: []string{"node1"},
			err: errors.New("synchronous standby string has number but lacks brackets"),
		},
	}

	for i, tt := range tests {
		out, err := parseSynchronousStandbyNames(tt.in)

		if tt.err != nil {
			if err == nil {
				t.Errorf("%d: got no error, wanted error: %v", i, tt.err)
			} else if tt.err.Error() != err.Error() {
				t.Errorf("%d: got error: %v, wanted error: %v", i, err, tt.err)
			}
		} else {
			if err != nil {
				t.Errorf("%d: unexpected error: %v", i, err)
			} else if !reflect.DeepEqual(out, tt.out) {
				t.Errorf("%d: wrong output: got:\n%s\nwant:\n%s", i, out, tt.out)
			}
		}
	}
}

func TestGenerateHBA(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	const (
		db1 = "db1"
		db2 = "db2"
		db3 = "db3"
	)

	// minimal clusterdata with only the fields used by generateHBA
	cd := &cluster.Data{
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

	const (
		defaultLine1 = "local postgres superuser md5"
		defaultLine2 = "local replication repluser md5"
	)
	tests := []struct {
		DefaultSUReplAccessMode cluster.SUReplAccessMode
		dbUID                   string
		pgHBA                   []string
		out                     []string
	}{
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessAll,
			dbUID:                   db1,
			out: []string{
				defaultLine1,
				defaultLine2,
				"host all superuser 0.0.0.0/0 md5",
				"host all superuser ::0/0 md5",
				"host replication repluser 0.0.0.0/0 md5",
				"host replication repluser ::0/0 md5",
				"host all all 0.0.0.0/0 md5",
				"host all all ::0/0 md5",
			},
		},
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessAll,
			dbUID:                   db2,
			out: []string{
				defaultLine1,
				defaultLine2,
				"host all superuser 0.0.0.0/0 md5",
				"host all superuser ::0/0 md5",
				"host replication repluser 0.0.0.0/0 md5",
				"host replication repluser ::0/0 md5",
				"host all all 0.0.0.0/0 md5",
				"host all all ::0/0 md5",
			},
		},
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessAll,
			dbUID:                   db1,
			pgHBA: []string{
				"host all all 192.168.0.0/24 md5",
			},
			out: []string{
				defaultLine1,
				defaultLine2,
				"host all superuser 0.0.0.0/0 md5",
				"host all superuser ::0/0 md5",
				"host replication repluser 0.0.0.0/0 md5",
				"host replication repluser ::0/0 md5",
				"host all all 192.168.0.0/24 md5",
			},
		},
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessAll,
			dbUID:                   db2,
			pgHBA: []string{
				"host all all 192.168.0.0/24 md5",
			},
			out: []string{
				defaultLine1,
				defaultLine2,
				"host all superuser 0.0.0.0/0 md5",
				"host all superuser ::0/0 md5",
				"host replication repluser 0.0.0.0/0 md5",
				"host replication repluser ::0/0 md5",
				"host all all 192.168.0.0/24 md5",
			},
		},
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessStrict,
			dbUID:                   db1,
			out: []string{
				defaultLine1,
				defaultLine2,
				"host all superuser 192.168.0.2/32 md5",
				"host replication repluser 192.168.0.2/32 md5",
				"host all superuser 192.168.0.3/32 md5",
				"host replication repluser 192.168.0.3/32 md5",
				"host all all 0.0.0.0/0 md5",
				"host all all ::0/0 md5",
			},
		},
		{
			DefaultSUReplAccessMode: cluster.SUReplAccessStrict,
			dbUID:                   db2,
			out: []string{
				defaultLine1,
				defaultLine2,
				"host all all 0.0.0.0/0 md5",
				"host all all ::0/0 md5",
			},
		},
	}

	for i, tt := range tests {
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

		cd.Cluster.Spec.DefaultSUReplAccessMode = &tt.DefaultSUReplAccessMode

		db := cd.DBs[tt.dbUID]
		db.Spec.PGHBA = tt.pgHBA

		out := p.generateHBA(ctx, cd, db, false)

		if !reflect.DeepEqual(out, tt.out) {
			var b bytes.Buffer
			b.WriteString(fmt.Sprintf("#%d: wrong output: got:\n", i))
			for _, o := range out {
				b.WriteString(fmt.Sprintf("%s\n", o))
			}
			b.WriteString("\nwant:\n")
			for _, o := range tt.out {
				b.WriteString(fmt.Sprintf("%s\n", o))
			}
			t.Error(b.String())
		}
	}
}

func TestGetTimeLinesHistory(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	t.Run("should return empty if timelineID is not greater than 1", func(t *testing.T) {
		pgState := &cluster.PostgresState{
			TimelineID: 1,
		}

		ctlsh, err := getTimeLinesHistory(ctx, pgState, nil, 3)

		if err != nil {
			t.Errorf("err should be nil, but got %v", err)
		}
		if len(ctlsh) != 0 {
			t.Errorf("expecting empty ctlsh, but got %v", ctlsh)
		}
	})

	t.Run("should return error if there is error in getting timeline history", func(t *testing.T) {
		var timelineID uint64 = 2
		pgState := &cluster.PostgresState{
			TimelineID: timelineID,
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		pgm := pgmocks.NewMockPGManager(ctrl)
		pgm.EXPECT().GetTimelinesHistory(timelineID).Return(
			[]*pg.TimelineHistory{},
			errors.New("failed to get timeline history"))
		ctlsh, err := getTimeLinesHistory(ctx, pgState, pgm, 3)

		if err == nil {
			t.Errorf("err should be not be nil")
		}
		if err != nil && err.Error() != "failed to get timeline history" {
			t.Errorf("err should be failed to get timeline history, but got %v", err)
		}
		if len(ctlsh) != 0 {
			t.Errorf("expecting empty ctlsh, but got %v", ctlsh)
		}
	})

	t.Run(
		"should return timeline history as is if the given length is less than maxPostgresTimelinesHistory",
		func(t *testing.T) {
			var timelineID uint64 = 2
			pgState := &cluster.PostgresState{
				TimelineID: timelineID,
			}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			pgm := pgmocks.NewMockPGManager(ctrl)
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

			if err != nil {
				t.Errorf("err should be not be nil")
			}
			if len(ctlsh) != 2 {
				t.Errorf("expecting length of ctlsh to be 2, but got %d", len(ctlsh))
			}
			expectedTimelineHistories := cluster.PostgresTimelinesHistory{
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
			}
			fmt.Println(ctlsh, expectedTimelineHistories)
			if *ctlsh[0] != *expectedTimelineHistories[0] {
				t.Errorf("expected %v, but got %v ", *expectedTimelineHistories[0], *ctlsh[0])
			}
			if *ctlsh[1] != *expectedTimelineHistories[1] {
				t.Errorf("expected %v, but got %v ", *expectedTimelineHistories[1], *ctlsh[1])
			}
		})

	t.Run(
		// revive:disable-next-line
		"should return timeline history with last maxPostgresTimelinesHistory elements if timeline history length is greater than maxPostgresTimelinesHistory",
		func(t *testing.T) {
			var timelineID uint64 = 4
			pgState := &cluster.PostgresState{
				TimelineID: timelineID,
			}

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			pgm := pgmocks.NewMockPGManager(ctrl)
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

			if err != nil {
				t.Errorf("err should be not be nil")
			}
			if len(ctlsh) != 2 {
				t.Errorf("expecting length of ctlsh to be 2, but got %d", len(ctlsh))
			}
			expectedTimelineHistories := cluster.PostgresTimelinesHistory{
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
			}
			fmt.Println(ctlsh, expectedTimelineHistories)
			if *ctlsh[0] != *expectedTimelineHistories[0] {
				t.Errorf("expected %v, but got %v ", *expectedTimelineHistories[0], *ctlsh[0])
			}
			if *ctlsh[1] != *expectedTimelineHistories[1] {
				t.Errorf("expected %v, but got %v ", *expectedTimelineHistories[1], *ctlsh[1])
			}
		})
}

// hbaTestKeeper returns a keeper with the user and auth settings used by the hba tests.
func hbaTestKeeper() *PostgresKeeper {
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
func hbaTestClusterData(mode cluster.SUReplAccessMode) *cluster.Data {
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

// TestGenerateHBAOnlyInternal checks the hba used while a synchronous primary waits for its
// sync standbys (onlyInternal=true): only the superuser and replication user entries are
// generated, so normal users are locked out, and the spec PGHBA entries are left out.
func TestGenerateHBAOnlyInternal(t *testing.T) {
	ctx := context.Background()
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
	tests := []struct {
		name  string
		mode  cluster.SUReplAccessMode
		dbUID string
		pgHBA []string
		out   []string
	}{
		{name: "all, primary", mode: cluster.SUReplAccessAll, dbUID: "db1", out: allEntries},
		{name: "all, replica", mode: cluster.SUReplAccessAll, dbUID: "db2", out: allEntries},
		{
			name: "all, primary, custom pghba", mode: cluster.SUReplAccessAll, dbUID: "db1",
			pgHBA: customHBA, out: allEntries,
		},
		{name: "strict, primary", mode: cluster.SUReplAccessStrict, dbUID: "db1", out: strictPrimaryEntries},
		{
			name: "strict, primary, custom pghba", mode: cluster.SUReplAccessStrict, dbUID: "db1",
			pgHBA: customHBA, out: strictPrimaryEntries,
		},
		{name: "strict, replica", mode: cluster.SUReplAccessStrict, dbUID: "db2", out: []string{local1, local2}},
		{
			name: "strict, replica, custom pghba", mode: cluster.SUReplAccessStrict, dbUID: "db2",
			pgHBA: customHBA, out: []string{local1, local2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cd := hbaTestClusterData(tt.mode)
			db := cd.DBs[tt.dbUID]
			db.Spec.PGHBA = tt.pgHBA
			out := hbaTestKeeper().generateHBA(ctx, cd, db, true)
			if !reflect.DeepEqual(out, tt.out) {
				t.Errorf("got:\n%s\nwant:\n%s", strings.Join(out, "\n"), strings.Join(tt.out, "\n"))
			}
			for _, e := range out {
				if strings.Contains(e, " all all ") || strings.Contains(e, "appuser") {
					t.Errorf("onlyInternal hba lets normal users in: %q", e)
				}
			}
			// the same input without the lockout opens up to normal users again
			open := hbaTestKeeper().generateHBA(ctx, cd, db, false)
			if len(open) <= len(out) {
				t.Errorf("hba without onlyInternal should have more entries than %v, got %v", out, open)
			}
		})
	}
}

// TestGenerateHBACertLocalAuth checks that cert, which postgres doesn't support for local
// connections, becomes peer in the local entries, while the host entries keep cert.
func TestGenerateHBACertLocalAuth(t *testing.T) {
	p := hbaTestKeeper()
	p.pgSUAuthMethod = authCert
	p.pgSULocalAuthMethod = authCert
	p.pgReplAuthMethod = authCert
	p.pgReplLocalAuthMethod = authCert
	cd := hbaTestClusterData(cluster.SUReplAccessAll)
	out := p.generateHBA(context.Background(), cd, cd.DBs["db1"], true)
	want := []string{
		"local postgres superuser peer",
		"local replication repluser peer",
		"host all superuser 0.0.0.0/0 cert",
		"host all superuser ::0/0 cert",
		"host replication repluser 0.0.0.0/0 cert",
		"host replication repluser ::0/0 cert",
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(out, "\n"), strings.Join(want, "\n"))
	}
}

func TestLocalAuthMethod(t *testing.T) {
	ctx := context.Background()
	for in, want := range map[string]string{
		authCert: authPeer, authMd5: authMd5, authTrust: authTrust, authPeer: authPeer, authIdent: authIdent,
	} {
		if got := localAuthMethod(ctx, in); got != want {
			t.Errorf("localAuthMethod(%q) = %q, want %q", in, got, want)
		}
	}
}

// keeperLogCtx returns a context whose keeper logger writes to buf, so a test can check the warnings.
func keeperLogCtx(buf *bytes.Buffer) context.Context {
	_, def := logging.GetLogComponent(context.Background(), logging.KeeperComponent)
	l := zerolog.New(buf).Level(def.GetLevel())
	return l.WithContext(context.Background())
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	// WriteFile applies the umask and keeps the mode of an existing file; set it explicitly
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestReadPasswordFromFile(t *testing.T) {
	const (
		tooOpen  = "password file permissions are too open"
		password = "secret"
	)
	dir := t.TempDir()

	t.Run("returns the content as is, trailing newline included", func(t *testing.T) {
		// The trailing newline is stripped later, by validateUsersConfig; see TestValidateUsersConfig.
		for _, content := range []string{password, "secret\n", "secret\r\n", "  sp ace\tx \n\n", ""} {
			path := filepath.Join(dir, "pw")
			writeFile(t, path, content, ownerRWPermisions)
			var buf bytes.Buffer
			got, err := readPasswordFromFile(keeperLogCtx(&buf), path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != content {
				t.Errorf("got %q, want %q", got, content)
			}
			if strings.Contains(buf.String(), tooOpen) {
				t.Errorf("unexpected warning for mode 0600: %s", buf.String())
			}
		}
	})

	t.Run("missing file", func(t *testing.T) {
		path := filepath.Join(dir, "missing")
		if _, err := readPasswordFromFile(context.Background(), path); err == nil ||
			!strings.Contains(err.Error(), path) {
			t.Errorf("expected an error naming %s, got %v", path, err)
		}
	})

	t.Run("directory", func(t *testing.T) {
		if _, err := readPasswordFromFile(context.Background(), dir); err == nil {
			t.Error("expected an error reading a directory")
		}
	})

	t.Run("modes", func(t *testing.T) {
		const ownerR = 0o400
		tests := []struct {
			mode os.FileMode
			warn bool
		}{
			{mode: ownerR},
			{mode: ownerRWPermisions},
			// only the owner: fine, whatever the owner bits
			{mode: ownerRWExecPermisions},
			// any bit for the group or others is too open
			{mode: 0o640, warn: true},
			{mode: ownerRWOtherRPermisions, warn: true},
			{mode: 0o666, warn: true},
			{mode: 0o466, warn: true},
			{mode: 0o404, warn: true},
			{mode: 0o444, warn: true},
		}
		for _, tt := range tests {
			path := filepath.Join(dir, fmt.Sprintf("pw-%o", tt.mode))
			writeFile(t, path, password, tt.mode)
			var buf bytes.Buffer
			got, err := readPasswordFromFile(keeperLogCtx(&buf), path)
			// a too open file is only warned about, it is still read
			if err != nil || got != password {
				t.Errorf("mode %#o: got %q, %v", tt.mode, got, err)
			}
			if warned := strings.Contains(buf.String(), tooOpen); warned != tt.warn {
				t.Errorf("mode %#o: warned=%v, want %v (log: %s)", tt.mode, warned, tt.warn, buf.String())
			}
		}
	})

	t.Run("symlink", func(t *testing.T) {
		// Kubernetes mounts secrets as symlinks: the mode of the target counts, not the one of the link.
		target := filepath.Join(dir, "target")
		writeFile(t, target, password, ownerRWPermisions)
		link := filepath.Join(dir, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		got, err := readPasswordFromFile(keeperLogCtx(&buf), link)
		if err != nil || got != password {
			t.Errorf("got %q, %v", got, err)
		}
		if strings.Contains(buf.String(), tooOpen) {
			t.Errorf("unexpected warning for a symlink to a 0600 file, log: %s", buf.String())
		}
		openTarget := filepath.Join(dir, "open-target")
		writeFile(t, openTarget, password, ownerRWOtherRPermisions)
		openLink := filepath.Join(dir, "open-link")
		if err := os.Symlink(openTarget, openLink); err != nil {
			t.Fatal(err)
		}
		buf.Reset()
		if _, err := readPasswordFromFile(keeperLogCtx(&buf), openLink); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), tooOpen) {
			t.Errorf("expected a warning for a symlink to a 0644 file, log: %s", buf.String())
		}
	})
}

// TestValidateUsersConfig checks the user, auth method and password rules of the keeper (stolon TestInitUsers)
func TestValidateUsersConfig(t *testing.T) {
	dir := t.TempDir()
	pwFile := func(name, content string) string {
		path := filepath.Join(dir, name)
		writeFile(t, path, content, ownerRWPermisions)
		return path
	}
	suFile := pwFile("su", "supass")
	suNLFile := pwFile("su-nl", "supass\n")
	replFile := pwFile("repl", "replpass")
	onlyNLFile := pwFile("only-nl", "\n")
	// valid returns the settings of the flag defaults, with passwords
	valid := func() *config {
		return &config{
			pgSUUsername: "postgres", pgSUAuthMethod: authMd5, pgSUConnType: connTypeHost, pgSUPassword: "supass",
			pgReplUsername: "repl", pgReplAuthMethod: authMd5, pgReplConnType: connTypeHost, pgReplPassword: "replpass",
		}
	}
	sameUser := func(c *config) { c.pgSUUsername, c.pgReplUsername = "pg", "pg" }

	tests := []struct {
		name   string
		change func(c *config)
		want   string // the error, or "" when the settings are valid
	}{
		{name: "valid with passwords", change: func(*config) {}},
		{name: "valid with password files", change: func(c *config) {
			c.pgSUPassword, c.pgSUPasswordFile, c.pgReplPassword, c.pgReplPasswordFile = "", suFile, "", replFile
		}},
		{name: "repl username required", want: "--pg-repl-username is required",
			change: func(c *config) { c.pgReplUsername = "" }},
		{name: "repl password required", want: "one of --pg-repl-password or --pg-repl-passwordfile is required",
			change: func(c *config) { c.pgReplPassword = "" }},
		{name: "only one repl password",
			want:   "only one of --pg-repl-password or --pg-repl-passwordfile must be provided",
			change: func(c *config) { c.pgReplPasswordFile = replFile }},
		{name: "su password required", want: "one of --pg-su-password or --pg-su-passwordfile is required",
			change: func(c *config) { c.pgSUPassword = "" }},
		{name: "only one su password",
			want:   "only one of --pg-su-password or --pg-su-passwordfile must be provided",
			change: func(c *config) { c.pgSUPasswordFile = suFile }},
		{name: "missing password file", want: "cannot read pg superuser password", change: func(c *config) {
			c.pgSUPassword, c.pgSUPasswordFile = "", filepath.Join(dir, "nope")
		}},
		{name: "same user, same password", change: func(c *config) {
			sameUser(c)
			c.pgSUPassword, c.pgReplPassword = "pass", "pass"
		}},
		{name: "same user, different password",
			want: "provided superuser name and replication user name are the same but provided passwords are different",
			change: func(c *config) {
				sameUser(c)
				c.pgSUPassword, c.pgReplPassword = "pass1", "pass2"
			}},
		{name: "same user, password file has a trailing newline", change: func(c *config) {
			sameUser(c)
			c.pgSUPassword, c.pgSUPasswordFile, c.pgReplPassword = "", suNLFile, "supass"
		}},
		{name: "same user, different auth methods",
			want: "do not support different auth methods when utilizing superuser for replication",
			change: func(c *config) {
				sameUser(c)
				c.pgSUAuthMethod, c.pgSUPassword = authCert, ""
			}},
		{name: "password file with only a newline", want: "superuser password is empty after removing trailing new line",
			change: func(c *config) { c.pgSUPassword, c.pgSUPasswordFile = "", onlyNLFile }},
		{name: "repl trust with password",
			want:   "can not utilize --pg-repl-auth-method trust together with --pg-repl-password",
			change: func(c *config) { c.pgReplAuthMethod = authTrust }},
		{name: "su trust with password", want: "can not utilize --pg-su-auth-method trust",
			change: func(c *config) { c.pgSUAuthMethod = authTrust }},
		{name: "su local trust with password", want: "can not utilize --pg-su-auth-method trust",
			change: func(c *config) { c.pgSULocalAuthMethod = authTrust }},
		{name: "trust without passwords", change: func(c *config) {
			c.pgSUAuthMethod, c.pgSUPassword, c.pgReplAuthMethod, c.pgReplPassword = authTrust, "", authTrust, ""
		}},
		{name: "cert without passwords", change: func(c *config) {
			c.pgSUAuthMethod, c.pgSUPassword, c.pgReplAuthMethod, c.pgReplPassword = authCert, "", authCert, ""
		}},
		{name: "peer is a valid auth method", change: func(c *config) { c.pgSULocalAuthMethod = authPeer }},
		{name: "invalid repl auth method", want: "--pg-repl-auth-method must be one of: md5, trust, cert, ident or peer",
			change: func(c *config) { c.pgReplAuthMethod = "bogus" }},
		{name: "invalid su auth method", want: "--pg-su-auth-method must be one of: md5, trust, cert, ident or peer",
			change: func(c *config) { c.pgSUAuthMethod = "bogus" }},
		{name: "invalid su connection type", want: "--pg-su-connection-type must be one of",
			change: func(c *config) { c.pgSUConnType = "tcp" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := valid()
			tt.change(c)
			err := validateUsersConfig(context.Background(), c)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %v, want an error containing %q", err, tt.want)
			}
		})
	}

	t.Run("fills the local auth methods and trims the passwords", func(t *testing.T) {
		c := valid()
		c.pgSUPassword, c.pgSUPasswordFile = "", suNLFile
		if err := validateUsersConfig(context.Background(), c); err != nil {
			t.Fatal(err)
		}
		if c.pgSULocalAuthMethod != authMd5 || c.pgReplLocalAuthMethod != authMd5 {
			t.Errorf("local auth methods: got %q and %q, want md5", c.pgSULocalAuthMethod, c.pgReplLocalAuthMethod)
		}
		if c.pgSUPassword != "supass" {
			t.Errorf("su password: got %q, want %q", c.pgSUPassword, "supass")
		}
	})
}

func TestDBLocalStateDeepCopy(t *testing.T) {
	var nilState *DBLocalState
	if nilState.DeepCopy() != nil {
		t.Error("DeepCopy of nil should be nil")
	}
	s := &DBLocalState{
		UID:              "uid",
		Generation:       3,
		Initializing:     true,
		InitPGParameters: common.Parameters{"shared_buffers": "128MB"},
	}
	c := s.DeepCopy()
	if c == s || !reflect.DeepEqual(c, s) {
		t.Fatalf("expected an equal copy, got %+v", c)
	}
	c.InitPGParameters["shared_buffers"] = "1GB"
	if s.InitPGParameters["shared_buffers"] != "128MB" {
		t.Error("changing the copy changed the original")
	}
	p := &PostgresKeeper{dbLocalState: s}
	if dc := p.dbLocalStateCopy(); dc == s || !reflect.DeepEqual(dc, s) {
		t.Errorf("dbLocalStateCopy should return an equal copy, got %+v", dc)
	}
}

func TestLocalStateFiles(t *testing.T) {
	dir := t.TempDir()
	p := &PostgresKeeper{cfg: &config{dataDir: dir}}

	if err := p.loadKeeperLocalState(); !os.IsNotExist(err) {
		t.Errorf("expected a not exist error, got %v", err)
	}
	if err := p.loadDBLocalState(); !os.IsNotExist(err) {
		t.Errorf("expected a not exist error, got %v", err)
	}

	kls := &KeeperLocalState{UID: "keeper1", ClusterUID: "cluster1"}
	p.keeperLocalState = kls
	if err := p.saveKeeperLocalState(); err != nil {
		t.Fatal(err)
	}
	p.keeperLocalState = nil
	if err := p.loadKeeperLocalState(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.keeperLocalState, kls) {
		t.Errorf("got %+v, want %+v", p.keeperLocalState, kls)
	}

	dbls := &DBLocalState{UID: "db1", Generation: 2, InitPGParameters: common.Parameters{"a": "b"}}
	if err := p.saveDBLocalState(dbls); err != nil {
		t.Fatal(err)
	}
	if p.dbLocalState == dbls || !reflect.DeepEqual(p.dbLocalState, dbls) {
		t.Errorf("saveDBLocalState should keep a copy in memory, got %+v", p.dbLocalState)
	}
	p.dbLocalState = nil
	if err := p.loadDBLocalState(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p.dbLocalState, dbls) {
		t.Errorf("got %+v, want %+v", p.dbLocalState, dbls)
	}

	for _, path := range []string{p.keeperLocalStateFilePath(), p.dbLocalStateFilePath()} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != ownerRWPermisions {
			t.Errorf("%s: mode %#o, want %#o", path, fi.Mode().Perm(), ownerRWPermisions)
		}
		writeFile(t, path, "{not json", ownerRWPermisions)
	}
	if err := p.loadKeeperLocalState(); err == nil {
		t.Error("expected an error loading a corrupt keeper state")
	}
	if err := p.loadDBLocalState(); err == nil {
		t.Error("expected an error loading a corrupt db state")
	}
}

func TestIsMaster(t *testing.T) {
	db := func(role common.Role, ft cluster.FollowType) *cluster.DB {
		return &cluster.DB{Spec: &cluster.DBSpec{Role: role, FollowConfig: &cluster.FollowConfig{Type: ft}}}
	}
	if !IsMaster(db(common.RolePrimary, "")) {
		t.Error("a primary is a master")
	}
	if IsMaster(db(common.RoleReplica, cluster.FollowTypeInternal)) {
		t.Error("an internal replica is not a master")
	}
	if !IsMaster(db(common.RoleReplica, cluster.FollowTypeExternal)) {
		t.Error("the replica of an external primary (standby cluster) is a master")
	}
	defer func() {
		if recover() == nil {
			t.Error("an undefined role should panic")
		}
	}()
	IsMaster(db(common.RoleUndefined, ""))
}

func TestUsePgrewind(t *testing.T) {
	tests := []struct {
		user, password, auth string
		dbUsePgrewind        bool
		want                 bool
	}{
		{user: "su", password: "pw", auth: authMd5, dbUsePgrewind: true, want: true},
		{user: "su", auth: authCert, dbUsePgrewind: true, want: true},
		{user: "su", password: "pw", auth: authMd5},
		{user: "su", auth: authMd5, dbUsePgrewind: true},
		{password: "pw", auth: authMd5, dbUsePgrewind: true},
	}
	for _, tt := range tests {
		p := &PostgresKeeper{pgSUUsername: tt.user, pgSUPassword: tt.password, pgSUAuthMethod: tt.auth}
		db := &cluster.DB{Spec: &cluster.DBSpec{UsePgrewind: tt.dbUsePgrewind}}
		if got := p.usePgrewind(db); got != tt.want {
			t.Errorf("%+v: got %v, want %v", tt, got, tt.want)
		}
	}
}

func TestWalKeepSettings(t *testing.T) {
	p := &PostgresKeeper{}
	tests := []struct {
		params   cluster.PGParameters
		segments int
		size     string
	}{
		{params: nil, segments: minWalKeepSegments, size: "128"},
		{params: cluster.PGParameters{"wal_keep_segments": "100", "wal_keep_size": "2GB"}, segments: 100, size: "2GB"},
		// lower than the minimum or invalid segments are ignored; wal_keep_size is returned as is
		{params: cluster.PGParameters{"wal_keep_segments": "2", "wal_keep_size": "1"}, segments: minWalKeepSegments,
			size: "1"},
		{params: cluster.PGParameters{"wal_keep_segments": "lots"}, segments: minWalKeepSegments, size: "128"},
	}
	for _, tt := range tests {
		db := &cluster.DB{Spec: &cluster.DBSpec{PGParameters: tt.params}}
		if got := p.walKeepSegments(db); got != tt.segments {
			t.Errorf("%v: walKeepSegments = %d, want %d", tt.params, got, tt.segments)
		}
		if got := p.walKeepSize(db); got != tt.size {
			t.Errorf("%v: walKeepSize = %q, want %q", tt.params, got, tt.size)
		}
	}
}

// fakePGManager returns a pg.Manager whose postgres binary reports the given version.
// With an empty version there is no postgres binary.
func fakePGManager(t *testing.T, version string) *pg.Manager {
	t.Helper()
	dir := t.TempDir()
	if version != "" {
		script := fmt.Sprintf("#!/bin/sh\necho 'postgres (PostgreSQL) %s'\n", version)
		writeFile(t, filepath.Join(dir, "postgres"), script, ownerRWExecPermisions)
	}
	return pg.NewManager(dir, dir, "", nil, nil, "", "", "", "", "", "", 0)
}

func TestWalLevelAndMandatoryPGParameters(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		version  string
		params   cluster.PGParameters
		walLevel string
		walKey   string
		walValue string
	}{
		{name: "no binary", walLevel: "hot_standby"},
		{name: "9.5", version: "9.5.25", walLevel: "hot_standby", walKey: "wal_keep_segments", walValue: "8"},
		{name: "12", version: "12.3", walLevel: "replica", walKey: "wal_keep_segments", walValue: "8"},
		{name: "12 logical", version: "12.3", params: cluster.PGParameters{"wal_level": "logical"},
			walLevel: "logical", walKey: "wal_keep_segments", walValue: "8"},
		{name: "16", version: "16.2", walLevel: "replica", walKey: "wal_keep_size", walValue: "128"},
		{name: "16 minimal is ignored", version: "16.2", params: cluster.PGParameters{"wal_level": "minimal"},
			walLevel: "replica", walKey: "wal_keep_size", walValue: "128"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &PostgresKeeper{pgm: fakePGManager(t, tt.version)}
			db := &cluster.DB{Spec: &cluster.DBSpec{PGParameters: tt.params}}
			if got := p.walLevel(ctx, db); got != tt.walLevel {
				t.Errorf("walLevel = %q, want %q", got, tt.walLevel)
			}
			want := common.Parameters{
				"unix_socket_directories": common.PgUnixSocketDirectories,
				"wal_level":               tt.walLevel,
				"hot_standby":             "on",
			}
			if tt.walKey != "" {
				want[tt.walKey] = tt.walValue
			}
			if got := p.mandatoryPGParameters(ctx, db); !reflect.DeepEqual(got, want) {
				t.Errorf("mandatoryPGParameters = %v, want %v", got, want)
			}
		})
	}
}

func TestCreatePGParameters(t *testing.T) {
	ctx := context.Background()
	p := &PostgresKeeper{
		pgm:             fakePGManager(t, "16.2"),
		pgListenAddress: "10.0.0.1",
		pgPort:          "5433",
		dbLocalState:    &DBLocalState{InitPGParameters: common.Parameters{"init_only": "x", "port": "1"}},
	}
	spec := func() *cluster.DBSpec {
		return &cluster.DBSpec{
			MaxStandbys:          5,
			AdditionalWalSenders: 4,
			PGParameters:         cluster.PGParameters{"work_mem": "8MB", "hot_standby": "off"},
		}
	}

	t.Run("managed parameters override the user ones", func(t *testing.T) {
		got := p.createPGParameters(ctx, &cluster.DB{Spec: spec()})
		want := common.Parameters{
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
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("include config and pg_rewind", func(t *testing.T) {
		s := spec()
		s.IncludeConfig = true
		s.UsePgrewind = true
		got := p.createPGParameters(ctx, &cluster.DB{Spec: s})
		if got["init_only"] != "x" || got["port"] != "5433" || got["wal_log_hints"] != "on" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("synchronous standbys", func(t *testing.T) {
		tests := []struct {
			sync, external []string
			syncRepl       bool
			want           string
		}{
			{syncRepl: true, want: ""},
			{sync: []string{"a"}, want: ""},
			{syncRepl: true, sync: []string{"a"}, want: common.OrionName("a")},
			{syncRepl: true, external: []string{"ext"}, want: "ext"},
			{syncRepl: true, sync: []string{"a", "b"}, external: []string{"ext"},
				want: fmt.Sprintf("3 (%s,%s,ext)", common.OrionName("a"), common.OrionName("b"))},
		}
		for _, tt := range tests {
			s := spec()
			s.SynchronousReplication = tt.syncRepl
			s.SynchronousStandbys = tt.sync
			s.ExternalSynchronousStandbys = tt.external
			got := p.createPGParameters(ctx, &cluster.DB{Spec: s})["synchronous_standby_names"]
			if got != tt.want {
				t.Errorf("%+v: got %q, want %q", tt, got, tt.want)
			}
			// what the keeper writes must parse back to the same standbys
			if got != "" {
				names, err := parseSynchronousStandbyNames(got)
				if err != nil || len(names) != len(tt.sync)+len(tt.external) {
					t.Errorf("parse %q: %v, %v", got, names, err)
				}
			}
		}
	})
}

func TestCreateRecoveryOptions(t *testing.T) {
	p := &PostgresKeeper{}
	t.Run("standby, no target", func(t *testing.T) {
		got := p.createRecoveryOptions(pg.RecoveryModeStandby,
			&cluster.StandbySettings{PrimaryConninfo: "host=x", PrimarySlotName: "slot", RecoveryMinApplyDelay: "5min"},
			&cluster.ArchiveRecoverySettings{RestoreCommand: "cp %f %p"},
			nil)
		want := &pg.RecoveryOptions{
			RecoveryMode: pg.RecoveryModeStandby,
			RecoveryParameters: common.Parameters{
				"primary_conninfo":         "host=x",
				"primary_slot_name":        "slot",
				"recovery_min_apply_delay": "5min",
				"restore_command":          "cp %f %p",
				"recovery_target_timeline": "latest",
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
	t.Run("empty settings are left out", func(t *testing.T) {
		got := p.createRecoveryOptions(pg.RecoveryModeStandby, &cluster.StandbySettings{}, nil, nil)
		want := common.Parameters{"recovery_target_timeline": "latest"}
		if !reflect.DeepEqual(got.RecoveryParameters, want) {
			t.Errorf("got %v, want %v", got.RecoveryParameters, want)
		}
	})
	t.Run("pitr target", func(t *testing.T) {
		got := p.createRecoveryOptions(pg.RecoveryModeRecovery, nil, nil, &cluster.RecoveryTargetSettings{
			RecoveryTarget:         "immediate",
			RecoveryTargetLsn:      "0/3000000",
			RecoveryTargetName:     "before",
			RecoveryTargetTime:     "2026-01-01 00:00:00",
			RecoveryTargetXid:      "1234",
			RecoveryTargetTimeline: "2",
		})
		want := common.Parameters{
			"recovery_target":          "immediate",
			"recovery_target_lsn":      "0/3000000",
			"recovery_target_name":     "before",
			"recovery_target_time":     "2026-01-01 00:00:00",
			"recovery_target_xid":      "1234",
			"recovery_target_timeline": "2",
			"recovery_target_action":   "promote",
		}
		if got.RecoveryMode != pg.RecoveryModeRecovery || !reflect.DeepEqual(got.RecoveryParameters, want) {
			t.Errorf("got %+v, want %v", got, want)
		}
	})
	t.Run("empty pitr target has no timeline", func(t *testing.T) {
		got := p.createRecoveryOptions(pg.RecoveryModeRecovery, nil, nil, &cluster.RecoveryTargetSettings{})
		want := common.Parameters{"recovery_target_action": "promote"}
		if !reflect.DeepEqual(got.RecoveryParameters, want) {
			t.Errorf("got %v, want %v", got.RecoveryParameters, want)
		}
	})
}

func TestConnParams(t *testing.T) {
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
	tests := []struct {
		name string
		get  func(p *PostgresKeeper) pg.ConnParams
		want pg.ConnParams
	}{
		{
			name: "su",
			get:  func(p *PostgresKeeper) pg.ConnParams { return p.getSUConnParams(db, followed) },
			want: pg.ConnParams{
				pg.ConnParamKeyUser: "su", pg.ConnParamKeyHost: "10.0.0.1", pg.ConnParamKeyPort: "5432",
				pg.ConnParamKeyAppName: appName, pg.ConnParamKeyDbName: defaultDatabase,
				pg.ConnParamKeySSLMode: "require",
			},
		},
		{
			name: "repl",
			get:  func(p *PostgresKeeper) pg.ConnParams { return p.getReplConnParams(db, followed) },
			want: pg.ConnParams{
				pg.ConnParamKeyUser: "repl", pg.ConnParamKeyHost: "10.0.0.1", pg.ConnParamKeyPort: "5432",
				pg.ConnParamKeyAppName: appName, pg.ConnParamKeySSLMode: "prefer",
			},
		},
		{
			name: "local",
			get:  func(p *PostgresKeeper) pg.ConnParams { return p.getLocalConnParams() },
			want: pg.ConnParams{
				pg.ConnParamKeyUser: "su", pg.ConnParamKeyHost: common.PgUnixSocketDirectories,
				pg.ConnParamKeyPort: "5433", pg.ConnParamKeyDbName: defaultDatabase,
			},
		},
		{
			name: "local repl",
			get:  func(p *PostgresKeeper) pg.ConnParams { return p.getLocalReplConnParams() },
			want: pg.ConnParams{
				pg.ConnParamKeyUser: "repl", pg.ConnParamKeyHost: common.PgUnixSocketDirectories,
				pg.ConnParamKeyPort: "5433",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// the password is only part of the conn params with md5
			if got := tt.get(newKeeper(authCert)); !got.Equals(tt.want) {
				t.Errorf("cert: got %v, want %v", got, tt.want)
			}
			want := tt.want.Copy()
			if tt.want[pg.ConnParamKeyUser] == "su" {
				want[pg.ConnParamKeyPassword] = "supw"
			} else {
				want[pg.ConnParamKeyPassword] = "replpw"
			}
			if got := tt.get(newKeeper(authMd5)); !got.Equals(want) {
				t.Errorf("md5: got %v, want %v", got, want)
			}
		})
	}
}

func TestIsDifferentTimelineBranch(t *testing.T) {
	ctx := context.Background()
	p := &PostgresKeeper{}
	history := func(switchPoints ...uint64) cluster.PostgresTimelinesHistory {
		h := cluster.PostgresTimelinesHistory{}
		for i, sp := range switchPoints {
			h = append(h, &cluster.PostgresTimelineHistory{TimelineID: uint64(i + 1), SwitchPoint: sp})
		}
		return h
	}
	tests := []struct {
		name          string
		followedTL    uint64
		followedHist  cluster.PostgresTimelinesHistory
		ourTL, ourPos uint64
		ourHist       cluster.PostgresTimelinesHistory
		want          bool
	}{
		{name: "followed is behind us", followedTL: 1, ourTL: 2, want: true},
		{name: "same timeline 1", followedTL: 1, ourTL: 1},
		{name: "same timeline, no history", followedTL: 2, ourTL: 2},
		{name: "same timeline, same switch point", followedTL: 2, followedHist: history(100), ourTL: 2,
			ourHist: history(100)},
		{name: "same timeline, different switch point", followedTL: 2, followedHist: history(100), ourTL: 2,
			ourHist: history(200), want: true},
		{name: "followed ahead, forked after our position", followedTL: 2, followedHist: history(100), ourTL: 1,
			ourPos: 50},
		{name: "followed ahead, forked at our position", followedTL: 2, followedHist: history(100), ourTL: 1,
			ourPos: 100},
		{name: "followed ahead, forked before our position", followedTL: 2, followedHist: history(100), ourTL: 1,
			ourPos: 150, want: true},
		{name: "followed ahead, no history", followedTL: 2, ourTL: 1, ourPos: 150},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			followed := &cluster.DB{Status: cluster.DBStatus{TimelineID: tt.followedTL, TimelinesHistory: tt.followedHist}}
			state := &cluster.PostgresState{TimelineID: tt.ourTL, XLogPos: tt.ourPos, TimelinesHistory: tt.ourHist}
			if got := p.isDifferentTimelineBranch(ctx, followed, state); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
