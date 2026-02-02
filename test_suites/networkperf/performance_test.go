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
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"golang.org/x/sync/errgroup"
)

const (
	iperfResultTimeout = 4 * time.Minute
)

func extractIperfResult(t *testing.T, machineTypeName string, rawResult string) (float64, error) {
	t.Helper()

	resultsArray := strings.Split(rawResult, " ")
	if len(resultsArray) < 6 {
		return 0, fmt.Errorf("invalid result format: %q", rawResult)
	}

	numericResult, err := strconv.ParseFloat(resultsArray[5], 64)
	if err != nil {
		return 0, err
	}

	units := resultsArray[6]
	if !strings.HasPrefix(units, "G") { // If the units aren't in Gbits/s, we automatically fail.
		return 0, fmt.Errorf("unknown unit of measurement %q", units)
	}
	return numericResult, nil
}

func waitForIperfResults(ctx context.Context, numInterfaces int) ([]string, error) {
	rawResultsPerIface := make([]string, numInterfaces)
	var wg errgroup.Group

	timeoutCtx, cancel := context.WithTimeout(ctx, iperfResultTimeout)
	defer cancel()

	for i := 0; i < numInterfaces; i++ {
		index := i
		wg.Go(func() error {
			for {
				// GetMetadata internally retries for about 15 seconds.
				result, err := utils.GetMetadata(timeoutCtx, "instance", "guest-attributes", "testing", fmt.Sprintf("results-%d", index))
				if err == nil {
					rawResultsPerIface[index] = result
					return nil
				}

				if timeoutCtx.Err() != nil {
					return fmt.Errorf("timeout waiting for iperf results on interface %d: %v", index, timeoutCtx.Err())
				}
				time.Sleep(5 * time.Second)
			}
		})
	}
	if err := wg.Wait(); err != nil {
		return nil, err
	}

	return rawResultsPerIface, nil
}

type testAttributes struct {
	numInterfaces  int
	machineType    string
	networkTier    string
	wantThroughput float64
}

func queryTestAttributes(ctx context.Context) (*testAttributes, error) {
	numInterfacesStr, err := utils.GetMetadata(ctx, "instance", "attributes", "num-parallel-tests")
	if err != nil {
		return nil, fmt.Errorf("getting num-parallel-tests: %w", err)
	}
	numInterfaces, err := strconv.Atoi(numInterfacesStr)
	if err != nil {
		return nil, fmt.Errorf("converting num-parallel-tests to int: %w", err)
	}

	expectedPerfString, err := utils.GetMetadata(ctx, "instance", "attributes", "expectedperf")
	if err != nil {
		return nil, fmt.Errorf("getting expectedperf: %w", err)
	}
	expectedPerfBase, err := strconv.ParseFloat(expectedPerfString, 64)
	if err != nil {
		return nil, fmt.Errorf("converting expectedperf to float: %w", err)
	}
	// Relax the expected performance target to 85% of line rate, since this is more
	// practically achievable.
	expectedPerf := 0.85 * expectedPerfBase

	machineTypePath, err := utils.GetMetadata(ctx, "instance", "machine-type")
	if err != nil {
		return nil, fmt.Errorf("getting machine-type: %w", err)
	}
	machineTypeSplit := strings.Split(machineTypePath, "/")
	machineType := machineTypeSplit[len(machineTypeSplit)-1]
	networkTier, err := utils.GetMetadata(ctx, "instance", "attributes", "network-tier")
	if err != nil {
		return nil, fmt.Errorf("getting network-tier: %w", err)
	}

	return &testAttributes{
		numInterfaces:  numInterfaces,
		machineType:    machineType,
		networkTier:    networkTier,
		wantThroughput: expectedPerf,
	}, nil
}

// TestNetworkPerformance doesn't actually run network performance tests, but rather checks that
// the iperf results are above the expected performance target.
func TestNetworkPerformance(t *testing.T) {
	attrs, err := queryTestAttributes(utils.Context(t))
	if err != nil {
		t.Fatalf("Querying test attributes: %v", err)
	}

	rawResultsPerIface, err := waitForIperfResults(utils.Context(t), attrs.numInterfaces)
	if err != nil {
		t.Fatalf("Waiting for iperf results: %v", err)
	}

	// Check if it matches the target for each interface.
	for i := 0; i < attrs.numInterfaces; i++ {
		resultPerf, err := extractIperfResult(t, attrs.machineType, rawResultsPerIface[i])
		if err != nil {
			t.Fatalf("Failed to extract iperf result for %q: %v", rawResultsPerIface[i], err)
		}
		if resultPerf < attrs.wantThroughput {
			t.Errorf(
				"Did not meet performance expectation for %q with network tier %q on interface %d. got: %v Gbps, want: %v Gbps",
				attrs.machineType,
				attrs.networkTier,
				i,
				resultPerf,
				attrs.wantThroughput,
			)
		} else {
			t.Logf("Machine type: %v, got: %v Gbps, want: %v Gbps", attrs.machineType, resultPerf, attrs.wantThroughput)
		}
	}
}
