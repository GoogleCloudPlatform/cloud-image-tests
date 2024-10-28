// Copyright 2024 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package acceleratorconfig

import (
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func TestA3UNicCount(t *testing.T) {
	ctx := utils.Context(t)
	var wantedCount = 8
	cmd := exec.CommandContext(ctx, "rdma", "-j", "dev")
	res, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, \"rdma -j dev\"): %v, want nil", err)
	}

	var data []map[string]any
	t.Logf("rdma -j dev output: %s", string(res))
	if err := json.Unmarshal(res, &data); err != nil {
		t.Fatalf("json.Unmarshal: %v, want nil ", err)
	}
	NicCount := 0
	for _, item := range data {
		if _, ok := item["ifindex"]; ok {
			NicCount++
		}
	}

	if NicCount != wantedCount {
		t.Fatalf("TestA3UNicCount: Got %v, want %v", NicCount, wantedCount)
	}
}
