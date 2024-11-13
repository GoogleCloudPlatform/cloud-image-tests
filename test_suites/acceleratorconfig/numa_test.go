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
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func TestA3UGPUNumaMapping(t *testing.T) {
	for i := 0; i < 8; i++ {
		filePath := fmt.Sprintf("/sys/class/drm/card%d/device/numa_node", i)
		res, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("os.Readfile(%s): %v, want nil", filePath, err)
		}
		numaMapping := strings.TrimSpace(string(res))
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

func TestA3UNICNumaMapping(t *testing.T) {
	ctx := utils.Context(t)
	for i := 0; i < 10; i++ {
		mac, err := utils.GetMetadata(ctx, "instance", "network-interfaces", fmt.Sprintf("%d", i), "mac")
		if err != nil {
			t.Fatalf("utils.GetMetadata(ctx, instance, network-interfaces, %d, mac) = err %v", i, err)
		}
		iface, err := utils.GetInterfaceByMAC(mac)
		if err != nil {
			t.Fatalf("utils.GetInterfaceByMAC(%s) = err %v", mac, err)
		}
		filePath := fmt.Sprintf("/sys/class/net/%s/device/numa_node", iface.Name)
		res, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("os.Readfile(%s): %v, want nil", filePath, err)
		}
		numaMapping := strings.TrimSpace(string(res))
		t.Logf("%s (index %d) NUMA node: %s", iface.Name, i, numaMapping)
		// Eth0 and 2-6 expected to be on numa node 0
		if i == 0 || (i > 1 && i < 6) {
			if numaMapping != "0" {
				t.Fatalf("TestA3UNICNumaMapping: %s (index %d) has numa node %v, want 0", iface.Name, i, numaMapping)
			}
		} else {
			if numaMapping != "1" {
				t.Fatalf("TestA3UNICNumaMapping: %s (index %d) numa node %v, want 1", iface.Name, i, numaMapping)
			}
		}
	}
}
