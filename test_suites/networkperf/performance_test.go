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

package networkperf

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func TestNetworkPerformance(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	// Check performance of the driver.
	var results string
	var err error
	for i := 0; i < 3; i++ {
		time.Sleep(time.Duration(i) * time.Second)
		results, err = utils.GetMetadata(ctx, "instance", "guest-attributes", "testing", "results")
		if err == nil {
			break
		}
		if i == 2 {
			t.Fatalf("Error : Test results not found. %v", err)
		}
	}

	// Get the performance target.
	expectedPerfString, err := utils.GetMetadata(ctx, "instance", "attributes", "expectedperf")
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	expectedPerf, err := strconv.Atoi(expectedPerfString)
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	expected := 0.85 * float64(expectedPerf)

	// Get machine type and network name for logging.
	machineType, err := utils.GetMetadata(ctx, "instance", "machine-type")
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	machineTypeSplit := strings.Split(machineType, "/")
	machineTypeName := machineTypeSplit[len(machineTypeSplit)-1]

	network, err := utils.GetMetadata(ctx, "instance", "attributes", "network-tier")
	if err != nil {
		t.Fatal(err)
	}

	// Find actual performance.
	resultsArray := strings.Split(results, " ")
	resultPerf, err := strconv.ParseFloat(resultsArray[5], 64)
	if err != nil {
		t.Fatalf("Error: %v", err)
	}

	// Check the units.
	units := resultsArray[6]
	if !strings.HasPrefix(units, "G") { // If the units aren't in Gbits/s, we automatically fail.
		t.Fatalf("Error: Wrong unit of measurement on machine type %s with network %s. Expected: %v Gbits/s, Actual: %v %s", machineTypeName, network, expected, resultPerf, units)
	}

	// Check if it matches the target.
	if resultPerf < expected {
		t.Fatalf("Error: Did not meet performance expectation on machine type %s with network %s. Expected: %v Gbits/s, Actual: %v Gbits/s", machineTypeName, network, expected, resultPerf)
	}
	t.Logf("Machine type: %v, Expected: %v Gbits/s, Actual: %v Gbits/s", machineTypeName, expected, resultPerf)
}
