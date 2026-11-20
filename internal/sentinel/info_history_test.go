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

package sentinel

import (
	"reflect"
	"testing"

	cluster "github.com/pgvillage-tools/orion/api/v1"
)

func TestInfoHistoriesDeepCopy(t *testing.T) {
	if KeeperInfoHistories(nil).DeepCopy() != nil || ProxyInfoHistories(nil).DeepCopy() != nil {
		t.Error("a copy of nil isn't nil")
	}

	kih := KeeperInfoHistories{"keeper1": {KeeperInfo: &cluster.KeeperInfo{UID: "keeper1"}, Seen: true, Timer: 5}}
	kc := kih.DeepCopy()
	if !reflect.DeepEqual(kc, kih) {
		t.Fatalf("copy %+v differs from %+v", kc, kih)
	}
	kc["keeper1"].KeeperInfo.UID = "changed"
	if kih["keeper1"].KeeperInfo.UID != "keeper1" {
		t.Error("the keeper info history copy shares its keeper info")
	}

	pih := ProxyInfoHistories{"proxy1": {ProxyInfo: &cluster.ProxyInfo{UID: "proxy1"}, Timer: 5}}
	pc := pih.DeepCopy()
	if !reflect.DeepEqual(pc, pih) {
		t.Fatalf("copy %+v differs from %+v", pc, pih)
	}
	pc["proxy1"].ProxyInfo.UID = "changed"
	if pih["proxy1"].ProxyInfo.UID != "proxy1" {
		t.Error("the proxy info history copy shares its proxy info")
	}
}
