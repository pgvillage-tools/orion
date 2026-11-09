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
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pgvillage-tools/orion/internal/common"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These tests use small shell scripts as fake postgres binaries (postgres, pg_ctl, initdb, ...), so neither a
// PostgreSQL installation nor a running server is needed. The connection parameters point to a unix socket dir that
// does not exist, so everything that needs a live database fails fast with a connection error.
var _ = Describe("Manager", func() {
	var (
		ctx     context.Context
		base    string
		binDir  string
		dataDir string
		walDir  string
		m       *Manager
	)

	// fakeBin writes an executable shell script with the specified body as binDir/name
	fakeBin := func(name string, body string) {
		Ω(os.WriteFile(filepath.Join(binDir, name), []byte("#!/bin/sh\n"+body+"\n"), uRWX)).To(Succeed())
	}
	// fakePostgres writes a fake postgres binary that reports the specified version for `postgres -V`. Otherwise it
	// starts like postgres does: it writes its pid into postmaster.pid in the data dir (passed as `-D datadir`).
	fakePostgres := func(version string) {
		fakeBin("postgres", strings.Join([]string{
			`if [ "$1" = "-V" ]; then echo "postgres (PostgreSQL) ` + version + `"; exit 0; fi`,
			`printf "%s\n" "$*" > "$2/args"`,
			`echo $$ > "$2/postmaster.pid"`,
			`sleep 1`,
		}, "\n"))
	}
	// fakePgCtl writes a fake pg_ctl binary. `pg_ctl status` exits with statusExit (0 is running, 3 is stopped), or
	// with 3 after a successful `pg_ctl stop` when stopWorks is true. Other subcommands exit with otherExit.
	fakePgCtl := func(statusExit int, stopWorks bool, otherExit int) {
		stopped := filepath.Join(binDir, "stopped")
		script := []string{
			`printf "%s\n" "$*" >> "` + filepath.Join(binDir, "pg_ctl.args") + `"`,
			`if [ "$1" = "status" ]; then`,
			`  if [ -e "` + stopped + `" ]; then exit 3; fi`,
			`  exit ` + strconv.Itoa(statusExit),
			`fi`,
		}
		if stopWorks {
			script = append(script, `if [ "$1" = "stop" ]; then touch "`+stopped+`"; fi`)
		}
		fakeBin("pg_ctl", strings.Join(append(script, `exit `+strconv.Itoa(otherExit)), "\n"))
	}
	readFile := func(path string) string {
		data, err := os.ReadFile(path)
		Ω(err).NotTo(HaveOccurred())
		return string(data)
	}
	writeFile := func(path string, content string) {
		Ω(os.MkdirAll(filepath.Dir(path), uRWX)).To(Succeed())
		Ω(os.WriteFile(path, []byte(content), uRW)).To(Succeed())
	}
	// initDataDir creates a data dir with all files and dirs that IsInitialized checks for the specified version
	initDataDir := func(version string) {
		Ω(os.MkdirAll(dataDir, uRWX)).To(Succeed())
		writeFile(filepath.Join(dataDir, "PG_VERSION"), version+"\n")
		dirs := []string{"base", "global", "pg_dynshmem", "pg_logical", "pg_multixact", "pg_notify", "pg_replslot",
			"pg_serial", "pg_snapshots", "pg_stat", "pg_stat_tmp", "pg_subtrans", "pg_tblspc", "pg_twophase"}
		if strings.HasPrefix(version, "9.") {
			dirs = append(dirs, "pg_clog", "pg_xlog")
		} else {
			dirs = append(dirs, "pg_xact", "pg_wal")
		}
		for _, dir := range dirs {
			Ω(os.MkdirAll(filepath.Join(dataDir, dir), uRWX)).To(Succeed())
		}
		writeFile(filepath.Join(dataDir, "global", "pg_control"), "")
	}
	newManager := func(wal string) *Manager {
		// a unix socket dir that does not exist
		noServer := ConnParams{
			ConnParamKeyHost:    filepath.Join(base, "nosocket"),
			ConnParamKeyPort:    "5432",
			ConnParamKeyUser:    "postgres",
			ConnParamKeyDbName:  "postgres",
			ConnParamKeySSLMode: "disable",
		}
		return NewManager(binDir, filepath.Join(base, "data"), wal, noServer, noServer.Copy(),
			"md5", "postgres", "supass", "md5", "repl", "replpass", time.Second)
	}

	BeforeEach(func() {
		ctx = context.Background()
		// On macOS TempDir itself sits below a symlink (/var -> /private/var), which moveWal would see as a moved
		// pg_wal
		var err error
		base, err = filepath.EvalSymlinks(GinkgoT().TempDir())
		Ω(err).NotTo(HaveOccurred())
		binDir = filepath.Join(base, "bin")
		dataDir = filepath.Join(base, "data", "postgres")
		walDir = filepath.Join(base, "wal")
		Ω(os.MkdirAll(binDir, uRWX)).To(Succeed())
		m = newManager("")
	})

	Context("in memory state", func() {
		It("is initialized by NewManager", func() {
			Ω(m.dataDir).To(Equal(dataDir))
			Ω(m.CurParameters()).To(BeEmpty())
			Ω(m.CurRecoveryOptions()).To(Equal(NewRecoveryOptions()))
			Ω(m.CurHba()).To(BeNil())
		})
		It("only exposes new parameters, recovery options and hba after updating the current ones", func() {
			params := common.Parameters{"max_connections": "100"}
			ro := NewRecoveryOptions()
			ro.RecoveryMode = RecoveryModeStandby
			ro.RecoveryParameters["primary_slot_name"] = "slot1"
			hba := []string{"local all all trust"}

			m.SetParameters(params)
			m.SetRecoveryOptions(ro)
			m.SetHba(hba)
			Ω(m.CurParameters()).To(BeEmpty())
			Ω(m.CurRecoveryOptions().RecoveryMode).To(Equal(RecoveryModeNone))
			Ω(m.CurHba()).To(BeNil())

			m.UpdateCurParameters()
			m.UpdateCurRecoveryOptions()
			m.UpdateCurHba()
			Ω(m.CurParameters()).To(Equal(params))
			Ω(m.CurRecoveryOptions()).To(Equal(ro))
			Ω(m.CurHba()).To(Equal(hba))

			// the current values are copies
			params["max_connections"] = "200"
			ro.RecoveryParameters["primary_slot_name"] = "slot2"
			hba[0] = "host all all 0.0.0.0/0 md5"
			Ω(m.CurParameters()).To(HaveKeyWithValue("max_connections", "100"))
			Ω(m.CurRecoveryOptions().RecoveryParameters).To(HaveKeyWithValue("primary_slot_name", "slot1"))
			Ω(m.CurHba()).To(Equal([]string{"local all all trust"}))
		})
		It("resets the recovery options when set to nil", func() {
			ro := NewRecoveryOptions()
			ro.RecoveryMode = RecoveryModeRecovery
			m.SetRecoveryOptions(ro)
			m.SetRecoveryOptions(nil)
			Ω(m.recoveryOptions).To(Equal(NewRecoveryOptions()))
		})
	})

	Context("writing config files", func() {
		BeforeEach(func() {
			Ω(os.MkdirAll(dataDir, uRWX)).To(Succeed())
		})
		standby := func(params common.Parameters) *RecoveryOptions {
			ro := NewRecoveryOptions()
			ro.RecoveryMode = RecoveryModeStandby
			ro.RecoveryParameters = params
			return ro
		}

		Context("writeConf", func() {
			It("writes the parameters with single quotes doubled", func() {
				m.SetParameters(common.Parameters{"archive_command": "cp '%p' /x"})
				Ω(m.writeConf(false, false)).To(Succeed())
				Ω(readFile(filepath.Join(dataDir, postgresConf))).To(Equal("archive_command = 'cp ''%p'' /x'\n"))
				stat, err := os.Stat(filepath.Join(dataDir, postgresConf))
				Ω(err).NotTo(HaveOccurred())
				Ω(stat.Mode().Perm()).To(Equal(os.FileMode(uRW)))
			})
			DescribeTable("writes the recovery parameters only when asked to, and not in recovery mode none",
				func(mode RecoveryMode, writeRecoveryParams bool, expected string) {
					ro := standby(common.Parameters{"primary_slot_name": "slot1"})
					ro.RecoveryMode = mode
					m.SetRecoveryOptions(ro)
					m.SetParameters(common.Parameters{"port": "5432"})
					Ω(m.writeConf(false, writeRecoveryParams)).To(Succeed())
					Ω(readFile(filepath.Join(dataDir, postgresConf))).To(Equal(expected))
				},
				Entry("standby, writing recovery params", RecoveryModeStandby, true,
					"port = '5432'\nprimary_slot_name = 'slot1'\n"),
				Entry("standby, not writing recovery params", RecoveryModeStandby, false, "port = '5432'\n"),
				Entry("recovery mode none", RecoveryModeNone, true, "port = '5432'\n"),
			)
			It("writes a temporary config that includes postgresql.conf when that exists", func() {
				m.SetParameters(common.Parameters{"port": "5432"})
				Ω(m.writeConf(true, false)).To(Succeed())
				Ω(readFile(filepath.Join(dataDir, tmpPostgresConf))).To(Equal("port = '5432'\n"))

				writeFile(filepath.Join(dataDir, postgresConf), "")
				Ω(m.writeConf(true, false)).To(Succeed())
				Ω(readFile(filepath.Join(dataDir, tmpPostgresConf))).
					To(Equal("include 'postgresql.conf'\nport = '5432'\n"))
			})
			It("fails when the data dir does not exist", func() {
				Ω(os.RemoveAll(dataDir)).To(Succeed())
				Ω(m.writeConf(false, false)).NotTo(Succeed())
			})
		})

		Context("writeRecoveryConf", func() {
			It("writes nothing in recovery mode none", func() {
				Ω(m.writeRecoveryConf()).To(Succeed())
				Ω(filepath.Join(dataDir, postgresRecoveryConf)).NotTo(BeAnExistingFile())
			})
			It("adds standby_mode for a standby", func() {
				m.SetRecoveryOptions(standby(common.Parameters{"primary_conninfo": "host=db1"}))
				Ω(m.writeRecoveryConf()).To(Succeed())
				Ω(readFile(filepath.Join(dataDir, postgresRecoveryConf))).
					To(Equal("standby_mode = 'on'\nprimary_conninfo = 'host=db1'\n"))
			})
			It("only writes the parameters for point in time recovery", func() {
				ro := standby(common.Parameters{"recovery_target_time": "2026-01-01"})
				ro.RecoveryMode = RecoveryModeRecovery
				m.SetRecoveryOptions(ro)
				Ω(m.writeRecoveryConf()).To(Succeed())
				Ω(readFile(filepath.Join(dataDir, postgresRecoveryConf))).
					To(Equal("recovery_target_time = '2026-01-01'\n"))
			})
		})

		DescribeTable("signal files are only written in their recovery mode",
			func(mode RecoveryMode, standbySignal bool, recoverySignal bool) {
				ro := NewRecoveryOptions()
				ro.RecoveryMode = mode
				m.SetRecoveryOptions(ro)
				Ω(m.writeStandbySignal(ctx)).To(Succeed())
				Ω(m.writeRecoverySignal(ctx)).To(Succeed())
				Ω(fileExists(filepath.Join(dataDir, postgresStandbySignal))).To(Equal(standbySignal))
				Ω(fileExists(filepath.Join(dataDir, postgresRecoverySignal))).To(Equal(recoverySignal))
			},
			Entry("none", RecoveryModeNone, false, false),
			Entry("standby", RecoveryModeStandby, true, false),
			Entry("recovery", RecoveryModeRecovery, false, true),
		)

		It("writes pg_hba.conf", func() {
			Ω(m.writePgHba()).To(Succeed())
			Ω(readFile(filepath.Join(dataDir, "pg_hba.conf"))).To(BeEmpty())
			m.SetHba([]string{"local all all trust", "host all all ::1/128 md5"})
			Ω(m.writePgHba()).To(Succeed())
			Ω(readFile(filepath.Join(dataDir, "pg_hba.conf"))).
				To(Equal("local all all trust\nhost all all ::1/128 md5\n"))
		})

		Context("createPostgresqlAutoConf", func() {
			It("replaces postgresql.auto.conf with a symlink to /dev/null", func() {
				autoConf := filepath.Join(dataDir, postgresAutoConf)
				writeFile(autoConf, "port = 5433\n")
				Ω(m.createPostgresqlAutoConf()).To(Succeed())
				Ω(os.Readlink(autoConf)).To(Equal("/dev/null"))
				// and it can be done again
				Ω(m.createPostgresqlAutoConf()).To(Succeed())
				Ω(os.Readlink(autoConf)).To(Equal("/dev/null"))
			})
			It("fails when the data dir does not exist", func() {
				Ω(os.RemoveAll(dataDir)).To(Succeed())
				Ω(m.createPostgresqlAutoConf()).To(MatchError(ContainSubstring("error symlinking")))
			})
			It("fails when postgresql.auto.conf cannot be removed", func() {
				writeFile(filepath.Join(dataDir, postgresAutoConf, "file"), "")
				Ω(m.createPostgresqlAutoConf()).To(MatchError(ContainSubstring("error removing")))
			})
		})

		Context("writeConfs", func() {
			BeforeEach(func() {
				m.SetParameters(common.Parameters{"port": "5432"})
				m.SetHba([]string{"local all all trust"})
				m.SetRecoveryOptions(standby(common.Parameters{"primary_slot_name": "slot1"}))
			})
			It("writes recovery parameters in postgresql.conf and a signal file for postgres 12 and newer", func() {
				fakePostgres("16.4")
				Ω(m.writeConfs(ctx, false)).To(Succeed())
				Ω(readFile(filepath.Join(dataDir, postgresConf))).To(ContainSubstring("primary_slot_name = 'slot1'"))
				Ω(readFile(filepath.Join(dataDir, "pg_hba.conf"))).To(Equal("local all all trust\n"))
				Ω(filepath.Join(dataDir, postgresStandbySignal)).To(BeARegularFile())
				Ω(filepath.Join(dataDir, postgresRecoveryConf)).NotTo(BeAnExistingFile())
			})
			It("writes recovery.conf for postgres 11 and older", func() {
				fakePostgres("11.22")
				Ω(m.writeConfs(ctx, false)).To(Succeed())
				Ω(readFile(filepath.Join(dataDir, postgresConf))).To(Equal("port = '5432'\n"))
				Ω(readFile(filepath.Join(dataDir, postgresRecoveryConf))).To(ContainSubstring("standby_mode = 'on'"))
				Ω(filepath.Join(dataDir, postgresStandbySignal)).NotTo(BeAnExistingFile())
			})
			It("fails when the postgres binary is missing", func() {
				Ω(m.writeConfs(ctx, false)).To(MatchError(ContainSubstring("error fetching pg version")))
			})
			It("fails when the postgres version cannot be parsed", func() {
				fakeBin("postgres", `echo "nothing useful"`)
				Ω(m.writeConfs(ctx, false)).To(MatchError(ContainSubstring("failed to parse postgres binary version")))
			})
			It("fails when the data dir does not exist", func() {
				fakePostgres("16.4")
				Ω(os.RemoveAll(dataDir)).To(Succeed())
				Ω(m.writeConfs(ctx, false)).To(MatchError(ContainSubstring("error writing postgresql.conf")))
			})
		})
	})

	Context("data dir state", func() {
		DescribeTable("IsInitialized",
			func(version string, remove string, expected bool) {
				initDataDir(version)
				if remove != "" {
					Ω(os.RemoveAll(filepath.Join(dataDir, remove))).To(Succeed())
				}
				Ω(m.IsInitialized()).To(Equal(expected))
			},
			Entry("postgres 16", "16", "", true),
			Entry("postgres 9.6", "9.6", "", true),
			Entry("postgres 16 without pg_wal", "16", "pg_wal", false),
			Entry("postgres 9.6 without pg_xlog", "9.6", "pg_xlog", false),
			Entry("without pg_control", "16", "global/pg_control", false),
			Entry("without PG_VERSION", "16", "PG_VERSION", false),
			Entry("without data dir", "16", ".", false),
		)
		It("IsInitialized fails on an invalid PG_VERSION", func() {
			initDataDir("16")
			writeFile(filepath.Join(dataDir, "PG_VERSION"), "invalid\n")
			_, err := m.IsInitialized()
			Ω(err).To(HaveOccurred())
		})
		It("reads PG_VERSION", func() {
			initDataDir("13")
			Ω(m.PGDataVersion()).To(Equal(V13))
		})

		DescribeTable("GetRole",
			func(version string, file string, expected common.Role) {
				fakePostgres(version)
				Ω(os.MkdirAll(dataDir, uRWX)).To(Succeed())
				if file != "" {
					writeFile(filepath.Join(dataDir, file), "")
				}
				Ω(m.GetRole()).To(Equal(expected))
			},
			Entry("postgres 16 primary", "16.4", "", common.RolePrimary),
			Entry("postgres 16 standby", "16.4", postgresStandbySignal, common.RoleReplica),
			Entry("postgres 16 with recovery.conf", "16.4", postgresRecoveryConf, common.RolePrimary),
			Entry("postgres 11 primary", "11.22", "", common.RolePrimary),
			Entry("postgres 11 standby", "11.22", postgresRecoveryConf, common.RoleReplica),
		)
		It("GetRole fails when the postgres binary is missing", func() {
			_, err := m.GetRole()
			Ω(err).To(MatchError(ContainSubstring("error fetching pg version")))
		})

		Context("GetSystemID", func() {
			It("reads the system id from pg_control", func() {
				pgControl := binary.NativeEndian.AppendUint64(nil, 7123456789012345678)
				Ω(os.MkdirAll(filepath.Join(dataDir, "global"), uRWX)).To(Succeed())
				Ω(os.WriteFile(filepath.Join(dataDir, "global", "pg_control"), pgControl, uRW)).To(Succeed())
				Ω(m.GetSystemID()).To(Equal("7123456789012345678"))
			})
			It("fails without pg_control", func() {
				_, err := m.GetSystemID()
				Ω(err).To(MatchError(os.ErrNotExist))
			})
			It("fails on a truncated pg_control", func() {
				writeFile(filepath.Join(dataDir, "global", "pg_control"), "abc")
				_, err := m.GetSystemID()
				Ω(err).To(HaveOccurred())
			})
		})

		Context("OlderWalFile", func() {
			walFile := func(dir string, name string, size int64) {
				path := filepath.Join(dataDir, dir, name)
				writeFile(path, "")
				Ω(os.Truncate(path, size)).To(Succeed())
			}
			It("returns the oldest wal file", func() {
				initDataDir("16")
				walFile("pg_wal", "000000010000000000000004", walSegSize)
				walFile("pg_wal", "000000010000000000000003", walSegSize)
				walFile("pg_wal", "00000001.history", 1)
				Ω(os.Mkdir(filepath.Join(dataDir, "pg_wal", "archive_status"), uRWX)).To(Succeed())
				Ω(m.OlderWalFile()).To(Equal("000000010000000000000003"))
			})
			It("uses pg_xlog for postgres 9.6", func() {
				initDataDir("9.6")
				walFile("pg_xlog", "000000010000000000000005", walSegSize)
				Ω(m.OlderWalFile()).To(Equal("000000010000000000000005"))
			})
			It("returns nothing without wal files", func() {
				initDataDir("16")
				Ω(m.OlderWalFile()).To(BeEmpty())
			})
			It("fails on wal files with an unsupported size", func() {
				initDataDir("16")
				walFile("pg_wal", "000000010000000000000003", 1024)
				_, err := m.OlderWalFile()
				Ω(err).To(MatchError(ContainSubstring("wal file has unsupported size: 1024")))
			})
			It("fails without a wal dir", func() {
				initDataDir("16")
				Ω(os.RemoveAll(filepath.Join(dataDir, "pg_wal"))).To(Succeed())
				_, err := m.OlderWalFile()
				Ω(err).To(MatchError(os.ErrNotExist))
			})
			It("fails without PG_VERSION", func() {
				_, err := m.OlderWalFile()
				Ω(err).To(HaveOccurred())
			})
		})

		Context("removing", func() {
			It("removes the data dir and the wal dir", func() {
				m = newManager(walDir)
				initDataDir("16")
				writeFile(filepath.Join(walDir, "000000010000000000000001"), "")
				Ω(m.RemoveAll(ctx)).To(Succeed())
				Ω(dataDir).NotTo(BeAnExistingFile())
				Ω(walDir).NotTo(BeAnExistingFile())
			})
			It("removes an uninitialized data dir", func() {
				writeFile(filepath.Join(dataDir, "leftover"), "")
				Ω(m.RemoveAllIfInitialized(ctx)).To(Succeed())
				Ω(dataDir).NotTo(BeAnExistingFile())
			})
			It("removes an initialized data dir when the instance is stopped", func() {
				initDataDir("16")
				fakePgCtl(exitStatusNotRunning, false, 0)
				Ω(m.RemoveAllIfInitialized(ctx)).To(Succeed())
				Ω(dataDir).NotTo(BeAnExistingFile())
			})
			It("does not remove the data dir of a running instance", func() {
				initDataDir("16")
				fakePgCtl(0, false, 0)
				Ω(m.RemoveAllIfInitialized(ctx)).To(MatchError(ContainSubstring("Instance is active")))
				Ω(dataDir).To(BeADirectory())
			})
			It("does not remove the data dir when the instance state is unknown", func() {
				initDataDir("16")
				Ω(m.RemoveAllIfInitialized(ctx)).To(MatchError(ContainSubstring("failed to retrieve instance state")))
				Ω(dataDir).To(BeADirectory())
			})
			It("does not remove the data dir with an invalid PG_VERSION", func() {
				initDataDir("16")
				writeFile(filepath.Join(dataDir, "PG_VERSION"), "invalid\n")
				Ω(m.RemoveAllIfInitialized(ctx)).To(MatchError(ContainSubstring("failed to retrieve instance state")))
				Ω(dataDir).To(BeADirectory())
			})
		})
	})

	Context("controlling the instance with pg_ctl", func() {
		pgCtlArgs := func() string { return readFile(filepath.Join(binDir, "pg_ctl.args")) }

		DescribeTable("IsStarted",
			func(statusExit int, expected bool, expectedErr error) {
				fakePgCtl(statusExit, false, 0)
				started, err := m.IsStarted()
				if expectedErr != nil {
					Ω(err).To(MatchError(expectedErr))
				} else {
					Ω(err).NotTo(HaveOccurred())
				}
				Ω(started).To(Equal(expected))
			},
			Entry("running", 0, true, nil),
			Entry("not running", exitStatusNotRunning, false, nil),
			Entry("inaccessible data dir", exitStatusInaccessibleDatadir, false, ErrInaccessibleDatadir),
		)
		It("IsStarted fails on other pg_ctl errors", func() {
			fakePgCtl(1, false, 0)
			_, err := m.IsStarted()
			Ω(err).To(MatchError(ContainSubstring("cannot get instance state")))
		})
		It("IsStarted fails without pg_ctl", func() {
			_, err := m.IsStarted()
			Ω(err).To(MatchError(ContainSubstring("cannot get instance state")))
		})

		It("stops", func() {
			fakePgCtl(0, true, 0)
			Ω(m.Stop(ctx, false)).To(Succeed())
			Ω(m.Stop(ctx, true)).To(Succeed())
			Ω(pgCtlArgs()).To(Equal(strings.Join([]string{
				"stop -w -D " + dataDir + " -o -c unix_socket_directories=/tmp",
				"stop -w -D " + dataDir + " -o -c unix_socket_directories=/tmp -m fast",
				"",
			}, "\n")))
		})
		It("fails to stop", func() {
			fakePgCtl(0, false, 1)
			Ω(m.Stop(ctx, false)).NotTo(Succeed())
		})

		DescribeTable("StopIfStarted",
			func(statusExit int, stopWorks bool, stopExit int, expectedErr string) {
				fakePgCtl(statusExit, stopWorks, stopExit)
				err := m.StopIfStarted(ctx, true)
				if expectedErr == "" {
					Ω(err).NotTo(HaveOccurred())
				} else {
					Ω(err).To(MatchError(ContainSubstring(expectedErr)))
				}
			},
			Entry("running and stopping", 0, true, 0, ""),
			Entry("not running", exitStatusNotRunning, false, 1, ""),
			Entry("inaccessible data dir", exitStatusInaccessibleDatadir, false, 1, ""),
			Entry("unknown state", 1, false, 0, "cannot get instance state"),
			Entry("failing to stop", 0, false, 1, "error: exit status 1"),
			Entry("still running after stop", 0, false, 0, "failed to stop"),
		)

		It("reloads and updates the current parameters", func() {
			fakePostgres("16.4")
			fakePgCtl(0, false, 0)
			initDataDir("16")
			m.SetParameters(common.Parameters{"work_mem": "8MB"})
			Ω(m.Reload(ctx)).To(Succeed())
			Ω(pgCtlArgs()).To(HavePrefix("reload -D " + dataDir))
			Ω(m.CurParameters()).To(Equal(common.Parameters{"work_mem": "8MB"}))
		})
		It("does not update the current parameters when reload fails", func() {
			fakePostgres("16.4")
			fakePgCtl(0, false, 1)
			initDataDir("16")
			m.SetParameters(common.Parameters{"work_mem": "8MB"})
			Ω(m.Reload(ctx)).NotTo(Succeed())
			Ω(m.CurParameters()).To(BeEmpty())
		})
		It("does not reload when the config cannot be written", func() {
			fakePgCtl(0, false, 0)
			Ω(m.Reload(ctx)).To(MatchError(ContainSubstring("error fetching pg version")))
			Ω(filepath.Join(binDir, "pg_ctl.args")).NotTo(BeAnExistingFile())
		})

		It("promotes and rewrites the config", func() {
			fakePostgres("16.4")
			fakePgCtl(0, false, 0)
			initDataDir("16")
			writeFile(filepath.Join(dataDir, postgresStandbySignal), "")
			m.SetParameters(common.Parameters{"port": "5432"})
			Ω(m.Promote(ctx)).To(Succeed())
			Ω(pgCtlArgs()).To(Equal("promote -w -D " + dataDir + "\n"))
			Ω(readFile(filepath.Join(dataDir, postgresConf))).To(Equal("port = '5432'\n"))
		})
		It("fails to promote", func() {
			fakePgCtl(0, false, 1)
			Ω(m.Promote(ctx)).NotTo(Succeed())
		})
	})

	Context("starting", func() {
		BeforeEach(func() {
			fakePostgres("16.4")
			initDataDir("16")
			m.SetParameters(common.Parameters{"port": "5432"})
		})
		It("writes the config, blocks postgresql.auto.conf and waits for postmaster.pid", func() {
			Ω(m.Start(ctx)).To(Succeed())
			Ω(readFile(filepath.Join(dataDir, postgresConf))).To(Equal("port = '5432'\n"))
			Ω(os.Readlink(filepath.Join(dataDir, postgresAutoConf))).To(Equal("/dev/null"))
			Ω(readFile(filepath.Join(dataDir, "args"))).
				To(Equal("-D " + dataDir + " -c unix_socket_directories=/tmp\n"))
			Ω(m.CurParameters()).To(Equal(common.Parameters{"port": "5432"}))
			Ω(filepath.Join(dataDir, "pg_wal")).To(BeADirectory())
		})
		It("starts with a temporary config", func() {
			Ω(m.StartTmpMerged(ctx)).To(Succeed())
			Ω(readFile(filepath.Join(dataDir, tmpPostgresConf))).To(Equal("port = '5432'\n"))
			Ω(readFile(filepath.Join(dataDir, "args"))).To(ContainSubstring(
				"-c config_file=" + filepath.Join(dataDir, tmpPostgresConf)))
		})
		It("restarts a stopped instance", func() {
			fakePgCtl(exitStatusNotRunning, false, 0)
			Ω(m.Restart(ctx, true)).To(Succeed())
			Ω(filepath.Join(dataDir, "postmaster.pid")).To(BeARegularFile())
		})
		It("does not start when stopping fails", func() {
			fakePgCtl(1, false, 0)
			Ω(m.Restart(ctx, true)).To(MatchError(ContainSubstring("cannot get instance state")))
			Ω(filepath.Join(dataDir, "postmaster.pid")).NotTo(BeAnExistingFile())
		})
		It("moves pg_wal to the wal dir, and back", func() {
			writeFile(filepath.Join(dataDir, "pg_wal", "000000010000000000000001"), "wal")
			m = newManager(walDir)
			Ω(m.Start(ctx)).To(Succeed())
			Ω(os.Readlink(filepath.Join(dataDir, "pg_wal"))).To(Equal(walDir))
			Ω(readFile(filepath.Join(walDir, "000000010000000000000001"))).To(Equal("wal"))

			m = newManager("")
			Ω(m.Start(ctx)).To(Succeed())
			Ω(filepath.Join(dataDir, "pg_wal")).To(BeADirectory())
			Ω(readFile(filepath.Join(dataDir, "pg_wal", "000000010000000000000001"))).To(Equal("wal"))
			Ω(filepath.Join(dataDir, "pg_wal_new")).NotTo(BeAnExistingFile())
		})
		It("does not start with a wal dir inside pg_wal", func() {
			m = newManager(filepath.Join(dataDir, "pg_wal", "sub"))
			Ω(m.Start(ctx)).To(MatchError(ContainSubstring("must not be (inside)")))
			Ω(filepath.Join(dataDir, "postmaster.pid")).NotTo(BeAnExistingFile())
		})
		It("does not start without pg_wal", func() {
			Ω(os.RemoveAll(filepath.Join(dataDir, "pg_wal"))).To(Succeed())
			Ω(m.Start(ctx)).To(MatchError(os.ErrNotExist))
		})
		It("does not start without a postgres binary", func() {
			Ω(os.Remove(filepath.Join(binDir, "postgres"))).To(Succeed())
			Ω(m.Start(ctx)).To(MatchError(ContainSubstring("error fetching pg version")))
			Ω(m.StartTmpMerged(ctx)).To(MatchError(ContainSubstring("error fetching pg version")))
		})
		It("fails when postgres exits without writing its pid", func() {
			fakeBin("postgres", `if [ "$1" = "-V" ]; then echo "postgres (PostgreSQL) 16.4"; exit 0; fi; exit 1`)
			Ω(m.Start(ctx)).To(MatchError("postgres exited unexpectedly"))
			Ω(m.CurParameters()).To(BeEmpty())
		})
		It("fails when postgres cannot be executed", func() {
			Ω(os.Remove(filepath.Join(binDir, "postgres"))).To(Succeed())
			Ω(m.start(ctx)).To(MatchError(ContainSubstring("error: ")))
		})
	})

	Context("initializing and restoring", func() {
		It("runs initdb with all options", func() {
			fakeBin("initdb", `printf "%s\n" "$*" > "`+filepath.Join(binDir, "initdb.args")+`"; mkdir -p "$2"`)
			m = newManager(walDir)
			Ω(m.Init(ctx, &InitConfig{Locale: "C", Encoding: "UTF8", DataChecksums: true})).To(Succeed())
			args := readFile(filepath.Join(binDir, "initdb.args"))
			Ω(args).To(HavePrefix("-D " + dataDir + " -U postgres --pwfile "))
			Ω(args).To(HaveSuffix(" --waldir " + walDir + " --locale C --encoding UTF8 --data-checksums\n"))
			Ω(dataDir).To(BeADirectory())
		})
		It("runs initdb without options", func() {
			fakeBin("initdb", `printf "%s\n" "$*" > "`+filepath.Join(binDir, "initdb.args")+`"`)
			m = NewManager(binDir, filepath.Join(base, "data"), "", ConnParams{}, ConnParams{},
				"trust", "postgres", "", "trust", "repl", "", time.Second)
			Ω(m.Init(ctx, &InitConfig{})).To(Succeed())
			Ω(readFile(filepath.Join(binDir, "initdb.args"))).To(Equal("-D " + dataDir + " -U postgres\n"))
		})
		It("removes a half initialized data dir when initdb fails", func() {
			fakeBin("initdb", `mkdir -p "$2"; exit 1`)
			Ω(m.Init(ctx, &InitConfig{})).NotTo(Succeed())
			Ω(dataDir).NotTo(BeAnExistingFile())
		})

		It("runs the restore command with the data dir and wal dir expanded", func() {
			m = newManager(walDir)
			Ω(m.Restore(ctx, `echo restored > %d/restored && echo "%w" > %d/waldir`)).To(Succeed())
			Ω(readFile(filepath.Join(dataDir, "restored"))).To(Equal("restored\n"))
			Ω(readFile(filepath.Join(dataDir, "waldir"))).To(Equal(walDir + "\n"))
		})
		It("removes the data dir when the restore command fails", func() {
			Ω(m.Restore(ctx, "touch %d/restored; exit 1")).NotTo(Succeed())
			Ω(dataDir).NotTo(BeAnExistingFile())
		})
		It("fails when the data dir cannot be created", func() {
			writeFile(filepath.Join(base, "data"), "a file blocking the data dir")
			Ω(m.Restore(ctx, "true")).To(MatchError(ContainSubstring("cannot create data dir")))
		})
	})

	Context("syncing from the followed instance", func() {
		// The fake binaries save their arguments and the pgpass file that they get through PGPASSFILE
		saveArgsAndPgPass := func(name string) {
			fakeBin(name, strings.Join([]string{
				`printf "%s\n" "$*" > "` + filepath.Join(binDir, name+".args") + `"`,
				`cat "$PGPASSFILE" > "` + filepath.Join(binDir, name+".pgpass") + `"`,
			}, "\n"))
		}
		followed := func() ConnParams {
			return ConnParams{
				ConnParamKeyHost:     "db1",
				ConnParamKeyPort:     "5433",
				ConnParamKeyUser:     "repl",
				ConnParamKeyPassword: "replpass",
			}
		}

		It("runs pg_basebackup with a slot and wal dir, and without the password in the connection string", func() {
			saveArgsAndPgPass("pg_basebackup")
			m = newManager(walDir)
			fcp := followed()
			Ω(m.SyncFromFollowed(ctx, fcp, "slot1")).To(Succeed())
			Ω(fcp).To(Equal(followed()))
			Ω(readFile(filepath.Join(binDir, "pg_basebackup.pgpass"))).To(Equal("db1:5433:*:repl:replpass\n"))
			args := readFile(filepath.Join(binDir, "pg_basebackup.args"))
			Ω(args).To(HavePrefix("-R -v -P -Xs -D " + dataDir + " -d "))
			Ω(args).To(ContainSubstring(`options=-c\ synchronous_commit=off`))
			Ω(args).NotTo(ContainSubstring("replpass"))
			Ω(args).To(HaveSuffix(" --slot slot1 --waldir " + walDir + "\n"))
		})
		It("runs pg_basebackup without slot and wal dir", func() {
			saveArgsAndPgPass("pg_basebackup")
			Ω(m.SyncFromFollowed(ctx, followed(), "")).To(Succeed())
			args := readFile(filepath.Join(binDir, "pg_basebackup.args"))
			Ω(args).NotTo(ContainSubstring("--slot"))
			Ω(args).NotTo(ContainSubstring("--waldir"))
		})
		It("fails when pg_basebackup fails", func() {
			fakeBin("pg_basebackup", "exit 1")
			Ω(m.SyncFromFollowed(ctx, followed(), "")).NotTo(Succeed())
		})
		It("fails without pg_basebackup", func() {
			Ω(m.SyncFromFollowed(ctx, followed(), "")).NotTo(Succeed())
		})

		It("runs pg_rewind after removing postgresql.auto.conf", func() {
			saveArgsAndPgPass("pg_rewind")
			Ω(os.MkdirAll(dataDir, uRWX)).To(Succeed())
			Ω(os.Symlink("/dev/null", filepath.Join(dataDir, postgresAutoConf))).To(Succeed())
			Ω(m.SyncFromFollowedPGRewind(ctx, followed(), "otherpass")).To(Succeed())
			Ω(filepath.Join(dataDir, postgresAutoConf)).NotTo(BeAnExistingFile())
			Ω(readFile(filepath.Join(binDir, "pg_rewind.pgpass"))).To(Equal("db1:5433:*:repl:otherpass\n"))
			Ω(readFile(filepath.Join(binDir, "pg_rewind.args"))).
				To(HavePrefix("--debug -D " + dataDir + " --source-server="))
		})
		It("fails when pg_rewind fails", func() {
			fakeBin("pg_rewind", "exit 1")
			Ω(m.SyncFromFollowedPGRewind(ctx, followed(), "")).NotTo(Succeed())
		})
		It("fails when postgresql.auto.conf cannot be removed", func() {
			writeFile(filepath.Join(dataDir, postgresAutoConf, "file"), "")
			Ω(m.SyncFromFollowedPGRewind(ctx, followed(), "")).To(MatchError(ContainSubstring("error removing")))
		})
	})

	Context("waiting for recovery", func() {
		It("is done when recovery.signal is gone for postgres 12 and newer", func() {
			fakePostgres("16.4")
			Ω(m.WaitRecoveryDone(time.Second)).To(Succeed())
		})
		It("times out while recovery.signal exists", func() {
			fakePostgres("16.4")
			writeFile(filepath.Join(dataDir, postgresRecoverySignal), "")
			Ω(m.WaitRecoveryDone(time.Millisecond)).To(MatchError("timeout waiting for db recovery"))
		})
		It("is done when recovery.done exists for postgres 11 and older", func() {
			fakePostgres("11.22")
			writeFile(filepath.Join(dataDir, postgresRecoveryDone), "")
			Ω(m.WaitRecoveryDone(time.Second)).To(Succeed())
		})
		It("times out while recovery.done does not exist", func() {
			fakePostgres("11.22")
			Ω(m.WaitRecoveryDone(time.Millisecond)).To(MatchError("timeout waiting for db recovery"))
		})
		It("fails without a postgres binary", func() {
			Ω(m.WaitRecoveryDone(time.Second)).To(MatchError(ContainSubstring("error fetching pg version")))
		})
	})

	Context("without a database server", func() {
		It("fails to ping and times out waiting for the database", func() {
			Ω(m.Ping(ctx)).NotTo(Succeed())
			Ω(m.WaitReady(ctx, time.Millisecond)).To(MatchError("timeout waiting for db ready"))
		})
		DescribeTable("SetupRoles returns the error of the failing step",
			func(suAuthMethod string, replUsername string, replAuthMethod string, expectedErr string) {
				m.suAuthMethod = suAuthMethod
				m.replUsername = replUsername
				m.replAuthMethod = replAuthMethod
				Ω(m.SetupRoles(ctx)).To(MatchError(ContainSubstring(expectedErr)))
			},
			Entry("replication superuser with trust", "trust", "postgres", "md5",
				"error adding replication role to superuser"),
			Entry("replication superuser with password", "md5", "postgres", "md5",
				"error adding replication role to superuser"),
			Entry("superuser with password", "md5", "repl", "md5", "error setting superuser password"),
			Entry("replication user with password", "trust", "repl", "md5", "error creating replication role"),
			Entry("replication user with trust", "trust", "repl", "trust", "error creating replication role"),
		)
		It("fails all other queries", func() {
			fakePostgres("16.4")
			initDataDir("16")
			_, err := m.GetSyncStandbys(ctx)
			Ω(err).To(HaveOccurred())
			_, err = m.GetReplicationSlots(ctx)
			Ω(err).To(HaveOccurred())
			Ω(m.CreateReplicationSlot(ctx, "slot1")).NotTo(Succeed())
			Ω(m.DropReplicationSlot(ctx, "slot1")).NotTo(Succeed())
			_, err = m.GetSystemData(ctx)
			Ω(err).To(HaveOccurred())
			_, err = m.GetTimelinesHistory(ctx, 1)
			Ω(err).To(HaveOccurred())
			_, err = m.GetConfigFilePGParameters(ctx)
			Ω(err).To(HaveOccurred())
			_, err = m.IsRestartRequired(ctx, []string{"shared_buffers"})
			Ω(err).To(HaveOccurred())
		})
		It("checks pending restarts differently before postgres 9.5", func() {
			fakePostgres("9.4.26")
			_, err := m.IsRestartRequired(ctx, []string{"shared_buffers"})
			Ω(err).To(HaveOccurred())
		})
		It("fails to get the replication slots without PG_VERSION", func() {
			_, err := m.GetReplicationSlots(ctx)
			Ω(err).To(MatchError(ContainSubstring("failed to read PG_VERSION")))
		})
		It("fails to check pending restarts without a postgres binary", func() {
			_, err := m.IsRestartRequired(ctx, nil)
			Ω(err).To(MatchError(ContainSubstring("error fetching pg version")))
		})
	})
})
