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
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func TestPCIETopology(t *testing.T) {
	ctx := utils.Context(t)
	out, err := exec.CommandContext(ctx, "lspci", "-tv", "-n").CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, lspci -tv -n).CombinedOutput() failed unexpectedly, err = %s", err)
	}
	t.Logf("lspci -tv -n output:\n%s", out)

	const nicDeviceID = "15b3:101e"
	gpuDeviceID := gpuDeviceID(ctx, t)
	var gpuAndNicTopo []string
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, gpuDeviceID) || strings.Contains(line, nicDeviceID) {
			gpuAndNicTopo = append(gpuAndNicTopo, line)
		}
	}

	if len(gpuAndNicTopo) != 16 {
		t.Fatalf("TestPCIE: expected 16 GPU and NIC devices, got len(gpuAndNicTopo) = %d \ngpuAndNicTopo = %v", len(gpuAndNicTopo), gpuAndNicTopo)
	}

	// A3U/A4 should have 4 relevant PCIe subtrees each containing two GPUs and then two NICs.
	for i := 0; i < 16; i += 4 {
		if !strings.Contains(gpuAndNicTopo[i], gpuDeviceID) {
			t.Errorf("TestPCIE: Expected the 1st device in the PCIe subtree to contain GPU device ID: %s, got: %s", gpuDeviceID, gpuAndNicTopo[i])
		}
		if !strings.Contains(gpuAndNicTopo[i+1], gpuDeviceID) {
			t.Errorf("TestPCIE: Expected the 2nd device in the PCIe subtree to contain GPU device ID: %s, got: %s", gpuDeviceID, gpuAndNicTopo[i+1])
		}
		if !strings.Contains(gpuAndNicTopo[i+2], nicDeviceID) {
			t.Errorf("TestPCIE: Expected the 3rd device in the PCIe subtree to contain NIC device ID: %s, got: %s", nicDeviceID, gpuAndNicTopo[i+2])
		}
		if !strings.Contains(gpuAndNicTopo[i+3], nicDeviceID) {
			t.Errorf("TestPCIE: Expected the 4th device in the PCIe subtree to contain NIC device ID: %s, got: %s", nicDeviceID, gpuAndNicTopo[i+3])
		}
	}
}

func gpuDeviceID(ctx context.Context, t *testing.T) string {
	t.Helper()
	machineType, err := utils.GetMetadata(ctx, "instance", "machine-type")
	if err != nil {
		t.Fatalf("Failed to get machine type from metadata server, err = %v", err)
	}
	machineTypeSplit := strings.Split(machineType, "/")
	if len(machineTypeSplit) == 0 {
		t.Fatalf("Unexpected machine type format: %s", machineType)
	}
	machineTypeName := machineTypeSplit[len(machineTypeSplit)-1]

	switch machineTypeName {
	case "a4-highgpu-8g":
		return "10de:2901"
	case "a3-ultragpu-8g":
		return "10de:2335"
	default:
		t.Fatalf("Unsupported machine type: %s", machineType)
		return ""
	}
}
