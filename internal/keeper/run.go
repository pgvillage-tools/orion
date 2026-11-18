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

package keeper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"

	"github.com/pgvillage-tools/orion/internal/logging"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Run runs a keeper with cfg (checked with ValidateUsersConfig) until ctx is done or the keeper fails. It creates
// the data directory and, unless DisableDataDirLocking, takes an exclusive lock on it.
func Run(ctx context.Context, cfg Config) error {
	ctx, logger := logging.GetLogComponent(ctx, logging.KeeperComponent)
	if err := os.MkdirAll(cfg.DataDir, ownerRWExecPermisions); err != nil {
		return fmt.Errorf("cannot create data dir: %w", err)
	}

	// Open (and create if needed) the lock file.
	// There is no need to clean up this file since we don't use the file as an actual lock. We get a lock
	// on the file. So the lock get released when our process stops.
	if !cfg.DisableDataDirLocking {
		lockFileName := filepath.Join(cfg.DataDir, "lock")
		lockFile, err := os.OpenFile(lockFileName, os.O_RDWR|os.O_CREATE, ownerRWOtherRPermisions)
		if err != nil {
			return fmt.Errorf("cannot take exclusive lock on data dir %s: %w", lockFileName, err)
		}
		defer func() {
			if err := lockFile.Close(); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("closing lock file failed")
			}
		}()

		// Get a lock on our lock file.
		ft := &syscall.Flock_t{
			Type:   syscall.F_WRLCK,
			Whence: int16(io.SeekStart),
			Start:  0,
			Len:    0, // Entire file.
		}
		if err = syscall.FcntlFlock(lockFile.Fd(), syscall.F_SETLK, ft); err != nil {
			return fmt.Errorf("cannot take exclusive lock on data dir %s: %w", lockFileName, err)
		}
		logger.Info().Msg("exclusive lock on data dir taken")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		shutdownSeconds.SetToCurrentTime()
	}()
	end := make(chan error)

	if cfg.MetricsListenAddress != "" {
		// a mux of its own, not http.DefaultServeMux: a second Run would register /metrics twice
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		go func() {
			if err := http.ListenAndServe(cfg.MetricsListenAddress, mux); err != nil {
				logger.Error().AnErr(logging.KeyErr, err).Msg("metrics http server error")
				cancel()
			}
		}()
	}

	p, err := NewPostgresKeeper(ctx, &cfg, end)
	if err != nil {
		return fmt.Errorf("cannot create keeper: %w", err)
	}
	go p.Start(ctx)

	if err := <-end; err != nil {
		return fmt.Errorf("keeper stopped with error: %w", err)
	}
	return nil
}
