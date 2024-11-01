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
	"fmt"
	"os"
	"testing"
)

func TestA3UGPUNumaMapping(t *testing.T) {
	for i := 0; i < 8; i++ {
		filePath := fmt.Sprintf("/sys/class/drm/card%d/device/numa_node", i)
		res, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("os.Readfile(%s): %v, want nil", filePath, err)
		}
		numaMapping := string(res)
		t.Logf("Card %d NUMA node: %s", i, numaMapping)
		if i < 4 {
			if numaMapping != "0" {
				t.Fatalf("TestA3UGPUNumaMapping: GPU %d has numa node %v, want 0", i, numaMapping)
			}
		} else {
			if numaMapping != "1" {
				t.Fatalf("TestA3UGPUNumaMapping: GPU %d has numa node %v, want 1", i, numaMapping)
			}
		}
	}
}
