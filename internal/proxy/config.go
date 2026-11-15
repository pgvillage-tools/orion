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

import "github.com/pgvillage-tools/orion/internal/store"

// Config is the configuration of a proxy
type Config struct {
	store.CommonConfig

	// ListenAddress and Port are where the proxy accepts connections for the primary
	ListenAddress string
	Port          string
	// StopListening makes the proxy stop listening when it can't reach the store (fencing)
	StopListening bool

	// KeepAliveIdle, KeepAliveCount and KeepAliveInterval set the tcp keepalive of the proxied connections
	// (seconds, a count, seconds); 0 keeps the default of the system
	KeepAliveIdle     int
	KeepAliveCount    int
	KeepAliveInterval int
}
