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

func (c *ClusterChecker) startProxy(ctx context.Context) error {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	c.proxyMutex.Lock()
	defer c.proxyMutex.Unlock()
	if c.pp != nil {
		return nil
	}

	logger.Info().Msg("Starting proxying")
	addr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort(c.listenAddress, c.port))
	if err != nil {
		return fmt.Errorf("error resolving tcp addr %q: %v", addr.String(), err)
	}

	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return fmt.Errorf("error listening on tcp addr %q: %v", addr.String(), err)
	}

	pp := tcpproxy.New(listener, keepAliveConfig(c.cfg))
	// the proxy outlives the check that starts it: it stops with stopServing
	serveCtx, stopProxy := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan struct{})
	c.pp = pp
	c.listener = listener
	c.stopProxy = stopProxy
	c.proxyDone = done

	go func() {
		defer close(done)
		if err := pp.Serve(serveCtx); err != nil {
			select {
			case c.proxyErrCh <- err:
			default:
				// an error is pending already: Start returns on that one
			}
		}
	}()

	return nil
}

// keepAliveConfig returns the tcp keepalive of the proxied connections in cfg; a setting of 0 keeps the default of the
// OS
func keepAliveConfig(cfg Config) net.KeepAliveConfig {
	// a negative setting leaves the setting of the OS
	count := cfg.KeepAliveCount
	if count == 0 {
		count = -1
	}
	seconds := func(n int) time.Duration {
		if n == 0 {
			return -1
		}
		return time.Duration(n) * time.Second
	}
	return net.KeepAliveConfig{
		Enable:   true,
		Idle:     seconds(cfg.KeepAliveIdle),
		Interval: seconds(cfg.KeepAliveInterval),
		Count:    count,
	}
}

// stopServing stops the proxy: it closes the listener and all connections
func (c *ClusterChecker) stopServing(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	c.proxyMutex.Lock()
	defer c.proxyMutex.Unlock()
	if c.pp != nil {
		logger.Info().Msg("Stopping listening")
		c.stopProxy()
		<-c.proxyDone
		c.pp = nil
		c.listener = nil
	}
}

// setDestination sets the address the proxy forwards to (nil: it closes the connections), and returns whether it
// changed
func (c *ClusterChecker) setDestination(addr *net.TCPAddr) bool {
	c.proxyMutex.Lock()
	defer c.proxyMutex.Unlock()
	if c.pp != nil {
		c.pp.SetDestination(addr)
	}
	dest := ""
	if addr != nil {
		dest = addr.String()
	}
	changed := dest != c.dest
	c.dest = dest
	return changed
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

// fence closes all connections and, with stopListening, stops listening (for example to avoid load balancers forward
// connections to us since we aren't ready or in a bad state): without a successful check the proxy can't know whether
// its master is still the master
func (c *ClusterChecker) fence(ctx context.Context) {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	logger.Warn().Msg("check timeout timer fired: closing all connections")
	c.setDestination(nil)
	if c.stopListening {
		c.stopServing(ctx)
	}
}

// timeout returns the proxy timeout
func (c *ClusterChecker) timeout() time.Duration {
	c.configMutex.Lock()
	defer c.configMutex.Unlock()
	return c.proxyTimeout
}

// Start runs the checks of the proxy until ctx is done or the proxy fails. When no check succeeded for the proxy
// timeout (a check failed, or still runs), the proxy fences itself; a check that runs longer than the proxy timeout is
// canceled.
func (c *ClusterChecker) Start(ctx context.Context) error {
	_, logger := logging.GetLogComponent(ctx, logging.ProxyComponent)
	// buffered, so a check that ends after Start returned doesn't block
	checkCh := make(chan error, 1)
	timerCh := time.NewTimer(0).C
	fenceTimer := time.NewTimer(c.timeout())
	defer fenceTimer.Stop()

	for {
		select {
		case <-timerCh:
			go func() {
				checkCtx, cancel := context.WithTimeout(ctx, c.timeout())
				defer cancel()
				checkCh <- c.Check(checkCtx)
			}()
		case err := <-checkCh:
			if err != nil {
				// don't report check ok since it returned an error
				logger.Warn().AnErr(logging.KeyErr, err).Msg("check function error")
			} else {
				logger.Debug().Msg("check ok")
				// the check was ok: the proxy timeout starts again (from the timeout of the cluster spec)
				fenceTimer.Reset(c.timeout())
			}
			c.configMutex.Lock()
			timerCh = time.NewTimer(c.proxyCheckInterval).C
			c.configMutex.Unlock()

		case <-fenceTimer.C:
			c.fence(ctx)

		case err := <-c.proxyErrCh:
			return fmt.Errorf("proxy error: %v", err)
		case <-ctx.Done():
			c.stopServing(ctx)
			return nil
		}
	}
}
