// Copyright 2026 PgVillage
// Copyright 2015 Sorint.lab
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

package sentinel

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"reflect"
	"sync"
	"time"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/store"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/davecgh/go-spew/spew"
)

const (
	fakeStandbyName = "orionfakestandby"

	// electionRetryInterval is the time to wait before restarting the election loop
	electionRetryInterval = 10 * time.Second
)

const (
	logDB             = "db"
	logKeyNewCD       = "newcd"
	logRecDB          = "receivedDB"
	logKeeper         = "keeper"
	logDbSysID        = "dbSystemdID"
	logMasterDB       = "masterDB"
	logMasterSystemID = "masterSystemID"
	logPrevSyncStdbys = "prevSynchronousStandbys"
	logSyncStdbys     = "SynchronousStandbys"
	logSyncStdbyDb    = "synchronousStandbyDB"
	logRequiredWal    = "requiredWAL"
	logOldMasterWal   = "olderWAL"
)

func (s *Sentinel) electionLoop(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	for {
		logger.Info().Msg("Trying to acquire sentinels leadership")
		electedCh, errCh := s.election.RunForElection(ctx)
		for {
			select {
			case elected := <-electedCh:
				s.leaderMutex.Lock()
				if elected {
					logger.Info().Msg("sentinel leadership acquired")
					s.leader = true
					s.leadershipCount++
				} else {
					if s.leader {
						logger.Info().Msg("sentinel leadership lost")
					}
					s.leader = false
				}
				s.leaderMutex.Unlock()

			case err := <-errCh:
				if err != nil {
					logger.Error().AnErr(logging.KeyErr, err).Msg("election loop error")

					// It's important to Stop() any on-going elections, as most stores will block
					// until all previous elections have completed. If we continue without stopping,
					// we run the risk of preventing any subsequent elections from successfully
					// electing a leader.
					s.election.Stop()
				}
				goto end
			case <-ctx.Done():
				logger.Debug().Msg("stopping election loop")
				s.election.Stop()
				return
			}
		}
	end:
		time.Sleep(electionRetryInterval)
	}
}

// syncRepl return whether to use synchronous replication based on the current
// cluster spec.
func (s *Sentinel) syncRepl(spec *cluster.Spec) bool {
	// a cluster standby role means our "master" will act as a cascading standby to
	// the other keepers, in this case we can't use synchronous replication
	return *spec.SynchronousReplication && *spec.Role == cluster.Primary
}

func (s *Sentinel) setSentinelInfo(ctx context.Context, ttl time.Duration) error {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	sentinelInfo := &cluster.SentinelInfo{
		UID: s.uid,
	}
	logger.Debug().Any("sentinelInfo", sentinelInfo).Msg("sentinelInfo dump")

	return s.e.SetSentinelInfo(ctx, sentinelInfo, ttl)
}

func (s *Sentinel) updateChangeTimes(cd, newcd *cluster.Data) {
	newcd.ChangeTime = time.Now()

	if !reflect.DeepEqual(newcd.Cluster, cd.Cluster) {
		newcd.Cluster.ChangeTime = time.Now()
	}

	for dbUID, db := range newcd.DBs {
		prevDB, ok := cd.DBs[dbUID]
		if !ok {
			db.ChangeTime = time.Now()
			continue
		}
		if !reflect.DeepEqual(db, prevDB) {
			db.ChangeTime = time.Now()
		}
	}

	for keeperUID, keeper := range newcd.Keepers {
		prevKeeper, ok := cd.Keepers[keeperUID]
		if !ok {
			keeper.ChangeTime = time.Now()
			continue
		}
		if !reflect.DeepEqual(keeper, prevKeeper) {
			keeper.ChangeTime = time.Now()
		}
	}

	if !reflect.DeepEqual(newcd.Proxy, cd.Proxy) {
		newcd.Proxy.ChangeTime = time.Now()
	}
}

// Sentinel is a sttruct to keep all Sentinel info
type Sentinel struct {
	uid string
	cfg *Config
	e   consensus.Store

	election consensus.Election
	end      chan bool

	lastLeadershipCount uint

	updateMutex sync.Mutex
	leader      bool
	// Used to determine if we lost and regained the leadership
	leadershipCount uint
	leaderMutex     sync.Mutex

	initialClusterSpec *cluster.Spec

	sleepInterval  time.Duration
	requestTimeout time.Duration

	// Make UIDFn settable to ease testing with reproducible UIDs
	UIDFn func() string
	// Make RandFn settable to ease testing with reproducible "random" numbers
	RandFn func(int) int

	keeperErrorTimers      map[string]int64
	dbErrorTimers          map[string]int64
	dbNotIncreasingXLogPos map[string]int64
	dbConvergenceInfos     map[string]*DBConvergenceInfo

	keeperInfoHistories KeeperInfoHistories
	proxyInfoHistories  ProxyInfoHistories
}

// NewSentinel retruns a new Sentinel object
func NewSentinel(ctx context.Context, uid string, cfg *Config, end chan bool) (*Sentinel, error) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	var initialClusterSpec *cluster.Spec
	if cfg.InitialClusterSpecFile != "" {
		configData, err := os.ReadFile(cfg.InitialClusterSpecFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read provided initial cluster config file: %v", err)
		}
		if err := json.Unmarshal(configData, &initialClusterSpec); err != nil {
			return nil, fmt.Errorf("cannot parse provided initial cluster config: %v", err)
		}
		logger.Debug().Any("initialClusterSpec", spew.Sdump(initialClusterSpec)).Msg("initialClusterSpec dump")
		if err := initialClusterSpec.Validate(); err != nil {
			return nil, fmt.Errorf("invalid initial cluster: %v", err)
		}
	}

	e, err := store.NewStore(ctx, &cfg.CommonConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	election, err := store.NewElection(ctx, &cfg.CommonConfig, uid)
	if err != nil {
		return nil, fmt.Errorf("cannot create election: %v", err)
	}

	return &Sentinel{
		uid:                uid,
		cfg:                cfg,
		e:                  e,
		election:           election,
		leader:             false,
		initialClusterSpec: initialClusterSpec,
		end:                end,
		UIDFn:              common.UID,
		// This is just to choose a pseudo random keeper so
		// use math.rand (no need for crypto.rand) without an
		// initial seed.
		RandFn: rand.Intn,

		sleepInterval:  cluster.DefaultSleepInterval,
		requestTimeout: cluster.DefaultRequestTimeout,
	}, nil
}

// Start starts a Sentinel
func (s *Sentinel) Start(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	endCh := make(chan struct{})

	timerCh := time.NewTimer(0).C

	go s.electionLoop(ctx)

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("stopping orion sentinel")
			s.end <- true
			return
		case <-timerCh:
			go func() {
				s.clusterSentinelCheck(ctx)
				endCh <- struct{}{}
			}()
		case <-endCh:
			timerCh = time.NewTimer(s.sleepInterval).C
		}
	}
}

func (s *Sentinel) leaderInfo() (bool, uint) {
	s.leaderMutex.Lock()
	defer s.leaderMutex.Unlock()
	return s.leader, s.leadershipCount
}

func (s *Sentinel) clusterSentinelCheck(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	s.updateMutex.Lock()
	defer s.updateMutex.Unlock()
	e := s.e

	cd, prevCDPair, err := e.GetClusterData(ctx)
	if err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg("error retrieving cluster data")
		return
	}
	if cd != nil {
		if cd.FormatVersion != cluster.CurrentCDFormatVersion {
			logger.Error().
				AnErr(logging.KeyErr, err).
				Uint64("version", cd.FormatVersion).
				Msg("unsupported clusterdata format version")
			return
		}
		if err = cd.Cluster.Spec.Validate(); err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg("clusterdata validation failed")
			return
		}
		if cd.Cluster != nil {
			s.sleepInterval = cd.Cluster.DefSpec().SleepInterval.Duration
			s.requestTimeout = cd.Cluster.DefSpec().RequestTimeout.Duration
		}
	}

	logger.Debug().Any("cd", spew.Sdump(cd)).Msg("cd dump")

	if cd == nil {
		// Cluster first initialization
		if s.initialClusterSpec == nil {
			logger.Info().Msg("no cluster data available, waiting for it to appear")
			return
		}
		c := cluster.NewCluster(s.UIDFn(), s.initialClusterSpec)
		logger.Info().Msg("writing initial cluster data")
		newcd := cluster.NewClusterData(c)
		logger.Debug().Any(logKeyNewCD, spew.Sdump(newcd)).Msg("newcd dump")
		if _, err = e.AtomicPutClusterData(ctx, newcd, nil); err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg("error saving cluster data")
		}
		return
	}

	if err = s.setSentinelInfo(ctx, 2*s.sleepInterval); err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg("cannot update sentinel info")
		return
	}

	keepersInfo, err := s.e.GetKeepersInfo(ctx)
	if err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg("cannot get keepers info")
		return
	}
	logger.Debug().Any("keepersInfo", spew.Sdump(keepersInfo)).Msg("keepersInfo dump")

	proxiesInfo, err := s.e.GetProxiesInfo(ctx)
	if err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg("failed to get proxies info")
		return
	}

	isLeader, leadershipCount := s.leaderInfo()
	if !isLeader {
		return
	}

	// detect if this is the first check after (re)gaining leadership
	firstRun := false
	if s.lastLeadershipCount != leadershipCount {
		firstRun = true
		s.lastLeadershipCount = leadershipCount
	}

	// if this is the first check after (re)gaining leadership reset all
	// the internal timers
	if firstRun {
		s.keeperErrorTimers = map[string]int64{}
		s.dbErrorTimers = map[string]int64{}
		s.dbNotIncreasingXLogPos = map[string]int64{}
		s.keeperInfoHistories = KeeperInfoHistories{}
		s.dbConvergenceInfos = map[string]*DBConvergenceInfo{}
		s.proxyInfoHistories = ProxyInfoHistories{}

		// Update db convergence timers since its the first run
		s.updateDBConvergenceInfos(cd)
	}

	newcd, newKeeperInfoHistories := s.updateKeepersStatus(ctx, cd, keepersInfo, firstRun)
	logger.Debug().Any(logKeyNewCD, spew.Sdump(newcd)).Msg("newcd dump after updateKeepersStatus")

	activeProxiesInfos := s.activeProxiesInfos(proxiesInfo)

	newcd, err = s.updateCluster(ctx, newcd, activeProxiesInfos)
	if err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg("failed to update cluster data")
		return
	}
	logger.Debug().Any(logKeyNewCD, spew.Sdump(newcd)).Msg("newcd dump after updateCluster")

	if newcd != nil {
		s.updateChangeTimes(cd, newcd)
		if _, err := e.AtomicPutClusterData(ctx, newcd, prevCDPair); err != nil {
			logger.Error().AnErr(logging.KeyErr, err).Msg("error saving clusterdata")
		}
	}

	// Save the new keeperInfoHistories only on successful cluster data
	// update or in the next run we'll think that the saved keeperInfo was
	// already applied.
	s.keeperInfoHistories = newKeeperInfoHistories

	// Update db convergence timers using the new cluster data
	s.updateDBConvergenceInfos(newcd)

	// We only update this metric when we've completed all actions in this method
	// successfully. That enables us to alert on when Sentinels are failing to
	// correctly sync.
	lastCheckSuccessSeconds.SetToCurrentTime()
}

// Run runs a sentinel with cfg until ctx is done
func Run(ctx context.Context, cfg Config) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.SentinelComponent)
	uid := common.UID()
	logger.Info().Str("uid", uid).Msg("sentinel uid")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	end := make(chan bool)

	if cfg.MetricsListenAddress != "" {
		// a mux of its own, not http.DefaultServeMux: a second Run would register /metrics twice
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		go func() {
			err := http.ListenAndServe(cfg.MetricsListenAddress, mux)
			if err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("metrics http server error")
				cancel()
			}
		}()
	}

	s, err := NewSentinel(ctx, uid, &cfg, end)
	if err != nil {
		return fmt.Errorf("cannot create sentinel: %w", err)
	}
	go s.Start(ctx)

	// Ensure we collect Sentinel metrics prior to providing Prometheus with an update
	// and unregister it when Run ends, so a next Run (e.g. in a test) can register its own
	defer mustRegisterSentinelCollector(s)()

	<-end
	return nil
}
