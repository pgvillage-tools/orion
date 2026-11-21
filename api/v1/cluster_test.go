package v1

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgvillage-tools/orion/internal/util"
)

var _ = Describe("Cluster", func() {
	When("Copying a Cluster", func() {
		var (
			orgCluster, newCluster *Cluster
		)
		BeforeEach(func() {
			orgCluster = randomCluster()
			newCluster = orgCluster.DeepCopy()
		})
		It("should be a copy", func() {
			newCluster.Generation = randomInt64()
			Ω(newCluster).NotTo(Equal(orgCluster))
		})
		It("should copy all fields", func() {
			Ω(newCluster).To(Equal(orgCluster))
		})
		It("should return nil when Cluster is nil", func() {
			var c *Cluster
			Ω(c.DeepCopy()).To(BeNil())
		})
		It("should return empty struct when Cluster is empty struct", func() {
			var c Cluster
			Ω(c.DeepCopy()).To(Equal(&Cluster{}))
		})
	})
	When("Requesting a cluster with a Default Spec", func() {
		It("should be a copy", func() {
			c := randomCluster()
			spec := c.DefSpec()
			spec.MaxStandbyLag = randomUInt32()
			Ω(c.Spec.MaxStandbyLag).NotTo(Equal(spec.MaxStandbyLag))
		})
		It("should set defaults for unspecified values", func() {
			c := Cluster{}
			spec := c.DefSpec()
			Ω(spec.SleepInterval).To(Equal(&Duration{Duration: DefaultSleepInterval}))
			Ω(spec.RequestTimeout).To(Equal(&Duration{Duration: DefaultRequestTimeout}))
			Ω(spec.ConvergenceTimeout).To(Equal(&Duration{Duration: DefaultConvergenceTimeout}))
			Ω(spec.InitTimeout).To(Equal(&Duration{Duration: DefaultInitTimeout}))
			Ω(spec.SyncTimeout).To(Equal(&Duration{Duration: DefaultSyncTimeout}))
			Ω(spec.DBWaitReadyTimeout).To(Equal(&Duration{Duration: DefaultDBWaitReadyTimeout}))
			Ω(spec.FailInterval).To(Equal(&Duration{Duration: DefaultFailInterval}))
			Ω(spec.DeadKeeperRemovalInterval).To(Equal(&Duration{Duration: DefaultDeadKeeperRemovalInterval}))
			Ω(spec.ProxyCheckInterval).To(Equal(&Duration{Duration: DefaultProxyCheckInterval}))
			Ω(spec.ProxyTimeout).To(Equal(&Duration{Duration: DefaultProxyTimeout}))
			Ω(spec.MaxStandbys).To(Equal(util.ToPtr(DefaultMaxStandbys)))
			Ω(spec.MaxStandbysPerSender).To(Equal(util.ToPtr(DefaultMaxStandbysPerSender)))
			Ω(spec.MaxStandbyLag).To(Equal(util.ToPtr(uint32(DefaultMaxStandbyLag))))
			Ω(*spec.MaxStandbyLag).To(Equal(uint32(1<<20)), "1MiB")
			Ω(spec.SynchronousReplication).To(Equal(util.ToPtr(DefaultSynchronousReplication)))
			Ω(spec.UsePgrewind).To(Equal(util.ToPtr(DefaultUsePgrewind)))
			Ω(spec.ResyncFastCheckpoint).To(Equal(util.ToPtr(DefaultResyncFastCheckpoint)))
			Ω(spec.MinSynchronousStandbys).To(Equal(util.ToPtr(DefaultMinSynchronousStandbys)))
			Ω(spec.MaxSynchronousStandbys).To(Equal(util.ToPtr(DefaultMaxSynchronousStandbys)))
			Ω(spec.AdditionalWalSenders).To(Equal(util.ToPtr(uint16(DefaultAdditionalWalSenders))))
			Ω(spec.MergePgParameters).To(Equal(util.ToPtr(DefaultMergePGParameter)))
			Ω(spec.DefaultSUReplAccessMode).To(Equal(util.ToPtr(DefaultSUReplAccess)))
			Ω(spec.Role).To(Equal(util.ToPtr(DefaultRole)))
			Ω(spec.AutomaticPgRestart).To(Equal(util.ToPtr(DefaultAutomaticPgRestart)))
		})
		It("should not set defaults for specified values", func() {
			c := randomCluster()
			spec := c.DefSpec()
			Ω(spec.SleepInterval).To(Equal(c.Spec.SleepInterval))
			Ω(spec.RequestTimeout).To(Equal(c.Spec.RequestTimeout))
			Ω(spec.ConvergenceTimeout).To(Equal(c.Spec.ConvergenceTimeout))
			Ω(spec.InitTimeout).To(Equal(c.Spec.InitTimeout))
			Ω(spec.SyncTimeout).To(Equal(c.Spec.SyncTimeout))
			Ω(spec.DBWaitReadyTimeout).To(Equal(c.Spec.DBWaitReadyTimeout))
			Ω(spec.FailInterval).To(Equal(c.Spec.FailInterval))
			Ω(spec.DeadKeeperRemovalInterval).To(Equal(c.Spec.DeadKeeperRemovalInterval))
			Ω(spec.ProxyCheckInterval).To(Equal(c.Spec.ProxyCheckInterval))
			Ω(spec.ProxyTimeout).To(Equal(c.Spec.ProxyTimeout))
			Ω(spec.MaxStandbys).To(Equal(c.Spec.MaxStandbys))
			Ω(spec.MaxStandbysPerSender).To(Equal(c.Spec.MaxStandbysPerSender))
			Ω(spec.MaxStandbyLag).To(Equal(c.Spec.MaxStandbyLag))
			Ω(spec.SynchronousReplication).To(Equal(c.Spec.SynchronousReplication))
			Ω(spec.UsePgrewind).To(Equal(c.Spec.UsePgrewind))
			Ω(spec.MinSynchronousStandbys).To(Equal(c.Spec.MinSynchronousStandbys))
			Ω(spec.MaxSynchronousStandbys).To(Equal(c.Spec.MaxSynchronousStandbys))
			Ω(spec.AdditionalWalSenders).To(Equal(c.Spec.AdditionalWalSenders))
			Ω(spec.MergePgParameters).To(Equal(c.Spec.MergePgParameters))
			Ω(spec.DefaultSUReplAccessMode).To(Equal(c.Spec.DefaultSUReplAccessMode))
			Ω(spec.Role).To(Equal(c.Spec.Role))
			Ω(spec.AutomaticPgRestart).To(Equal(c.Spec.AutomaticPgRestart))
		})
	})
	When("Copying a Spec", func() {
		var (
			orgSpec, newSpec *Spec
		)
		BeforeEach(func() {
			orgSpec = randomSpec()
			newSpec = orgSpec.DeepCopy()
		})
		It("should be a copy", func() {
			newSpec.SleepInterval = randomDuration()
			Ω(newSpec).NotTo(Equal(orgSpec))
		})
		It("should copy all fields", func() {
			Ω(newSpec).To(Equal(orgSpec))
		})
		It("should return nil when Spec is nil", func() {
			var s *Spec
			Ω(s.DeepCopy()).To(BeNil())
		})
		It("should return empty struct when Spec is empty struct", func() {
			var s Spec
			Ω(s.DeepCopy()).To(Equal(&Spec{}))
		})
	})
	When("Validating a Spec", func() {
		It("should return no error when fields are unset", func() {
			var spec Spec
			Ω(spec.Validate()).To(HaveOccurred())
		})
		It("accepts a valid spec", func() {
			spec := Spec{InitMode: util.ToPtr(New)}
			Ω(spec.Validate()).To(Succeed())
		})
		It("returns the error of the setting that is wrong", func() {
			invalidDuration := &Duration{time.Duration(-1)}
			zero := util.ToPtr(uint16(0))
			for _, tt := range []struct {
				change func(s *Spec)
				want   string
			}{
				{func(s *Spec) { s.SleepInterval = invalidDuration }, "sleepInterval must be positive"},
				{func(s *Spec) { s.RequestTimeout = invalidDuration }, "requestTimeout must be positive"},
				{func(s *Spec) { s.ConvergenceTimeout = invalidDuration }, "convergenceTimeout must be positive"},
				{func(s *Spec) { s.InitTimeout = invalidDuration }, "initTimeout must be positive"},
				{func(s *Spec) { s.SyncTimeout = invalidDuration }, "syncTimeout must be positive"},
				{func(s *Spec) { s.DBWaitReadyTimeout = invalidDuration }, "dbWaitReadyTimeout must be positive"},
				{func(s *Spec) { s.FailInterval = invalidDuration }, "failInterval must be positive"},
				{func(s *Spec) { s.DeadKeeperRemovalInterval = invalidDuration },
					"deadKeeperRemovalInterval must be positive"},
				{func(s *Spec) { s.ProxyCheckInterval = invalidDuration }, "proxyCheckInterval must be positive"},
				{func(s *Spec) { s.ProxyTimeout = invalidDuration }, "proxyTimeout must be positive"},
				{func(s *Spec) { s.ProxyCheckInterval = &Duration{time.Minute} },
					"proxyCheckInterval should be less than proxyTimeout"},
				{func(s *Spec) { s.MaxStandbys = zero }, "maxStandbys must be at least 1"},
				{func(s *Spec) { s.MaxStandbysPerSender = zero }, "maxStandbysPerSender must be at least 1"},
				{func(s *Spec) { s.MaxSynchronousStandbys = zero }, "maxSynchronousStandbys must be at least 1"},
				{func(s *Spec) { s.MinSynchronousStandbys = util.ToPtr(uint16(2)) },
					"maxSynchronousStandbys must be greater or equal to minSynchronousStandbys"},
				{func(s *Spec) { s.InitMode = nil }, "initMode undefined"},
				{func(s *Spec) { s.AdditionalMasterReplicationSlots = []string{"slot-1"} },
					`wrong replication slot name: "slot-1"`},
				{func(s *Spec) { s.AdditionalMasterReplicationSlots = []string{"orion_slot"} },
					`replication slot name is reserved: "orion_slot"`},
				{func(s *Spec) { s.PGHBA = []string{"local all all md5\n"} },
					"pgHBA entries cannot contain newline characters"},
				{func(s *Spec) { s.InitMode = util.ToPtr(InitMode("invalid")) }, `unknown initMode: "invalid"`},
				{func(s *Spec) { s.Role = util.ToPtr(Replica) }, `invalid cluster role standby when initMode is "new"`},
				{func(s *Spec) { s.InitMode = util.ToPtr(ExistingCluster) },
					`existingConfig undefined. Required when initMode is "existing"`},
				{func(s *Spec) {
					s.InitMode, s.ExistingConfig = util.ToPtr(ExistingCluster), &ExistingConfig{}
				}, "existingConfig.keeperUID undefined"},
				{func(s *Spec) { s.InitMode = util.ToPtr(PITR) }, `pitrConfig undefined. Required when initMode is "pitr"`},
				{func(s *Spec) { s.InitMode, s.PITRConfig = util.ToPtr(PITR), &PITRConfig{} },
					"pitrConfig.DataRestoreCommand undefined"},
				{func(s *Spec) {
					s.InitMode, s.Role = util.ToPtr(PITR), util.ToPtr(Replica)
					s.PITRConfig = &PITRConfig{DataRestoreCommand: "restore", RecoveryTargetSettings: &RecoveryTargetSettings{}}
				}, "cannot define pitrConfig.RecoveryTargetSettings when required cluster role is standby"},
				{func(s *Spec) { s.DefaultSUReplAccessMode = util.ToPtr(SUReplAccessMode("invalid")) },
					`unknown defaultSUReplAccessMode: "invalid"`},
				{func(s *Spec) {
					s.InitMode, s.Role = util.ToPtr(PITR), util.ToPtr(Replica)
					s.PITRConfig = &PITRConfig{DataRestoreCommand: "restore"}
				}, `standbyConfig undefined. Required when cluster role is "standby"`},
				{func(s *Spec) { s.Role = util.ToPtr(Role("invalid")) }, `unknown role: "invalid"`},
			} {
				spec := Spec{InitMode: util.ToPtr(New)}
				tt.change(&spec)
				Ω(spec.Validate()).To(MatchError(tt.want))
			}
		})
	})
	When("Updating a Spec", func() {
		It("should succeed when all is good", func() {
			var (
				initMode = New
				specRole = Primary
				spec     = Spec{InitMode: &initMode, Role: &specRole}
				cl       = Cluster{Spec: &Spec{InitMode: &initMode, Role: &specRole}}
			)
			Ω(cl.UpdateSpec(&spec)).Error().NotTo(HaveOccurred())
		})
		It("should return an error when the new spec is invalid", func() {
			var (
				initMode = New
				specRole = Primary
				spec     = Spec{InitMode: nil}
				cl       = Cluster{Spec: &Spec{InitMode: &initMode, Role: &specRole}}
			)
			Ω(cl.UpdateSpec(&spec).Error()).To(Equal("invalid cluster spec: initMode undefined"))
		})
		It("should return an error when initMode changes", func() {
			var (
				specInitMode = PITR
				spec         = Spec{InitMode: &specInitMode,
					PITRConfig: &PITRConfig{DataRestoreCommand: "/bin/true"}}
				clInitMode = New
				cl         = Cluster{Spec: &Spec{InitMode: &clInitMode}}
			)
			Ω(cl.UpdateSpec(&spec).Error()).To(Equal("cannot change cluster init mode"))
		})
		It("should return an error when Role changes", func() {
			var (
				initMode       = ExistingCluster
				specRole       = Replica
				existingConfig = ExistingConfig{KeeperUID: randomString()}
				standbyConfig  = StandbyConfig{StandbySettings: &StandbySettings{},
					ArchiveRecoverySettings: &ArchiveRecoverySettings{}}
				spec = Spec{
					InitMode:       &initMode,
					Role:           &specRole,
					ExistingConfig: &existingConfig,
					StandbyConfig:  &standbyConfig,
				}
				clRole = Primary
				cl     = Cluster{Spec: &Spec{
					InitMode:       &initMode,
					Role:           &clRole,
					ExistingConfig: &existingConfig,
				}}
			)
			Ω(cl.UpdateSpec(&spec).Error()).To(Equal(
				"cannot update a cluster from master role to standby role"))
		})
	})
})
