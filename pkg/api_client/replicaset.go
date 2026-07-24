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
// See the License for the specific lang

package api_client

import (
	endpoints "github.com/pgvillage-tools/orion/internal/api_endpoints"
)

// PutPromoteReplicaSet uses the API to write replicaset config
func (c Connection) PutPromoteReplicaSet() (httpCode int, err error) {
	return c.Put(endpoints.PromoteReplicaSetEndPoint, nil)
}
