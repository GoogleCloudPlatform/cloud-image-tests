// Copyright 2024 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package shapevalidation

import (
	"strconv"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func TestMem(t *testing.T) {
	expectedMemory, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "expected_memory")
	if err != nil {
		t.Fatalf("could not get expected memory from metadata: %v", err)
	}
	emem, err := strconv.ParseUint(expectedMemory, 10, 64)
	if err != nil {
		t.Fatalf("could not parse uint64 from %s", expectedMemory)
	}
	mem, err := memTotal()
	if err != nil {
		t.Fatal(err)
	}
	if mem < emem {
		t.Errorf("got %d GB memory, want at least %d GB", mem, emem)
	}
}

func TestCpu(t *testing.T) {
	expectedCPU, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "expected_cpu")
	if err != nil {
		t.Fatalf("could not get expected cpu count from metadata: %v", err)
	}
	ecpu, err := strconv.Atoi(expectedCPU)
	if err != nil {
		t.Fatalf("could not parse int from %s", expectedCPU)
	}
	cpu, err := numCpus()
	if err != nil {
		t.Fatal(err)
	}
	if cpu != ecpu {
		t.Errorf("got %d CPUs want %d", cpu, ecpu)
	}
}

func TestNuma(t *testing.T) {
	expectedNuma, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "expected_numa")
	if err != nil {
		t.Fatalf("could not get expected numa node count from metadata: %v", err)
	}
	enuma, err := strconv.ParseUint(expectedNuma, 10, 8)
	if err != nil {
		t.Fatalf("could not parse uint8 from %s", expectedNuma)
	}
	numa, err := numNumaNodes()
	if err != nil {
		t.Fatal(err)
	}
	if !reliableNuma() {
		t.Skip("numa node counts are not reliable on this VM/OS combination")
	}
	if numa != uint8(enuma) {
		t.Errorf("got %d numa nodes, want %d", numa, uint8(enuma))
	}
}
