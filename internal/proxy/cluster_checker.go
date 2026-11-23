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

package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	cluster "github.com/pgvillage-tools/orion/api/v1"
	"github.com/pgvillage-tools/orion/internal/common"
	"github.com/pgvillage-tools/orion/internal/consensus"
	"github.com/pgvillage-tools/orion/internal/logging"
	"github.com/pgvillage-tools/orion/internal/proxy/tcpproxy"
	"github.com/pgvillage-tools/orion/internal/store"
	"github.com/pgvillage-tools/orion/internal/util"
	"github.com/rs/zerolog"

	"github.com/davecgh/go-spew/spew"
)

// ClusterChecker is struct containing information for checking the cluster
type ClusterChecker struct {
	uid           string
	listenAddress string
	port          string

	stopListening bool

	listener *net.TCPListener
	pp       *tcpproxy.Proxy
	// stopProxy stops pp, proxyDone is closed when it stopped
	stopProxy context.CancelFunc
	proxyDone chan struct{}
	e         consensus.Store
	// proxyErrCh gets the error of a proxy that stopped by itself
	proxyErrCh chan error

	proxyMutex sync.Mutex
	// dest is the address the proxy forwards to ("" when it doesn't), as last set
	dest string

	proxyCheckInterval time.Duration
	proxyTimeout       time.Duration
	configMutex        sync.Mutex

	cfg Config
}

// NewClusterChecker is a function which creates a new clusterchecker
func NewClusterChecker(ctx context.Context, uid string, cfg Config) (*ClusterChecker, error) {
	e, err := store.NewStore(ctx, &cfg.CommonConfig)
	if err != nil {
		return nil, fmt.Errorf("cannot create store: %v", err)
	}

	return &ClusterChecker{
		uid:           uid,
		listenAddress: cfg.ListenAddress,
		port:          cfg.Port,
		stopListening: cfg.StopListening,
		cfg:           cfg,
		e:             e,
		proxyErrCh:    make(chan error, 1),

		proxyCheckInterval: cluster.DefaultProxyCheckInterval,
		proxyTimeout:       cluster.DefaultProxyTimeout,
	}, nil
}

// changeEvent returns an info event when the address the proxy forwards to changed, and a debug event otherwise:
// every check repeats the same decision, but an operator must see when it changes
func changeEvent(logger *zerolog.Logger, changed bool) *zerolog.Event {
	if changed {
		return logger.Info()
	}
	return logger.Debug()
}

// SetProxyInfo is function which sets the proxy-info
func (c *ClusterChecker) SetProxyInfo(ctx context.Context, _ consensus.Store, generation int64,
	proxyTimeout time.Duration) error {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	proxyInfo := &cluster.ProxyInfo{
		InfoUID:      common.UID(),
		UID:          c.uid,
		Generation:   generation,
		ProxyTimeout: proxyTimeout,
	}
	logger.Debug().Str("proxyInfo dump", spew.Sdump(proxyInfo)).Msg("")

	return c.e.SetProxyInfo(ctx, proxyInfo, 2*proxyTimeout)
}

// Check reads the cluster data and sets the destination of the proxy.
func (c *ClusterChecker) Check(ctx context.Context) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	cd, _, err := c.e.GetClusterData(ctx)
	if err != nil {
		return fmt.Errorf("cannot get cluster data: %v", err)
	}

	// Start the proxy if not active
	if err = c.startProxy(ctx); err != nil {
		return fmt.Errorf("failed to start proxy: %v", err)
	}

	logger.Debug().Str("cd dump", spew.Sdump(cd)).Msg("")
	if cd == nil {
		changed := c.setDestination(nil)
		changeEvent(logger, changed).Msg("no clusterdata available, closing connections to master")
		return nil
	}
	if cd.FormatVersion != cluster.CurrentCDFormatVersion {
		c.setDestination(nil)
		return fmt.Errorf("unsupported clusterdata format version: %d", cd.FormatVersion)
	}
	if err = cd.Cluster.Spec.Validate(); err != nil {
		c.setDestination(nil)
		return fmt.Errorf("clusterdata validation failed: %v", err)
	}

	cdProxyCheckInterval := cd.Cluster.DefSpec().ProxyCheckInterval.Duration
	cdProxyTimeout := cd.Cluster.DefSpec().ProxyTimeout.Duration

	// use the greater between the current proxy timeout and the one defined in the cluster spec if they're different.
	// in this way we're updating our proxyInfo using a timeout that is greater
	// or equal the current active timeout timer.
	c.configMutex.Lock()
	proxyTimeout := c.proxyTimeout
	if cdProxyTimeout > proxyTimeout {
		proxyTimeout = cdProxyTimeout
	}
	c.configMutex.Unlock()

	proxy := cd.Proxy
	if proxy == nil {
		changed := c.setDestination(nil)
		changeEvent(logger, changed).Msg("no proxy object available, closing connections to master")
		// ignore errors on setting proxy info
		if err = c.SetProxyInfo(ctx, c.e, cluster.NoGeneration, proxyTimeout); err != nil {
			logger.Warn().AnErr(logging.KeyErr, err).Msg("failed to update proxyInfo")
		} else {
			// update proxyCheckinterval and proxyTimeout only if we successfully updated our proxy info
			c.configMutex.Lock()
			c.proxyCheckInterval = cdProxyCheckInterval
			c.proxyTimeout = cdProxyTimeout
			c.configMutex.Unlock()
		}
		return nil
	}

	db, ok := cd.DBs[proxy.Spec.MasterDBUID]
	if !ok {
		changed := c.setDestination(nil)
		changeEvent(logger, changed).Str("db", proxy.Spec.MasterDBUID).
			Msg("no db object available, closing connections to master")
		// ignore errors on setting proxy info
		if err = c.SetProxyInfo(ctx, c.e, proxy.Generation, proxyTimeout); err != nil {
			logger.Warn().AnErr(logging.KeyErr, err).Msg("failed to update proxyInfo")
		} else {
			// update proxyCheckinterval and proxyTimeout only if we successfully updated our proxy info
			c.configMutex.Lock()
			c.proxyCheckInterval = cdProxyCheckInterval
			c.proxyTimeout = cdProxyTimeout
			c.configMutex.Unlock()
		}
		return nil
	}

	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(db.Status.ListenAddress, db.Status.Port))
	if err != nil {
		logger.Error().AnErr(logging.KeyErr, err).Msg("cannot resolve db address")
		c.setDestination(nil)
		return nil
	}
	logger.Debug().Any("address", addr).Msg("master address")
	if err = c.SetProxyInfo(ctx, c.e, proxy.Generation, proxyTimeout); err != nil {
		// if we failed to update our proxy info when a master is defined we
		// cannot ignore this error since the sentinel won't know that we exist
		// and are sending connections to a master so, when electing a new
		// master, it'll not wait for us to close connections to the old one.
		return fmt.Errorf("failed to update proxyInfo: %v", err)
	}
	// update proxyCheckinterval and proxyTimeout only if we successfully updated our proxy info
	c.configMutex.Lock()
	c.proxyCheckInterval = cdProxyCheckInterval
	c.proxyTimeout = cdProxyTimeout
	c.configMutex.Unlock()

	// start proxing only if we are inside enabledProxies, this ensures that the
	// sentinel has read our proxyinfo and knows we are alive
	if util.StringInSlice(proxy.Spec.EnabledProxies, c.uid) {
		changed := c.setDestination(addr)
		changeEvent(logger, changed).Any("address", addr).Msg("proxying to master address")
	} else {
		changed := c.setDestination(nil)
		changeEvent(logger, changed).
			Any("address", addr).
			Msg("not proxying to master address since we aren't in the enabled proxies list")
	}

	return nil
}
