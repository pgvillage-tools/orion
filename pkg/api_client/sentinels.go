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

package api_client

import (
	apiv1 "github.com/pgvillage-tools/orion/api/v1"
	endpoints "github.com/pgvillage-tools/orion/pkg/api_endpoints"
)

// GetSentinelsStatus uses the API to fetch the info of the sentinels
func (c Connection) GetSentinelsStatus() (sentinels apiv1.SentinelsInfo, httpCode int, err error) {
	httpCode, err = c.Get(endpoints.SentinelStatusEndPoint, &sentinels)
	return sentinels, httpCode, err
}
