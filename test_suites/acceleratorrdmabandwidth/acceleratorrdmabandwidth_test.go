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

package acceleratorrdmabandwidth

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/acceleratorutils"
)

const (
	a3uAndA4LineRate     = 400                    // A3U and A4 line rate is 400 Gbps.
	expectedMinBandwidth = a3uAndA4LineRate * 0.8 // 320 Gbps
)

var ibWriteBWArgs = []string{
	"--report_gbits",
	"--iters=10000",
	"--size=65536",
	"--perform_warm_up",
}

func runIbWriteBWHost(ctx context.Context, t *testing.T, nic string) {
	t.Helper()
	args := append(ibWriteBWArgs, fmt.Sprintf("--ib-dev=%s", nic))
	command := exec.CommandContext(ctx, "./ib_write_bw", args...)
	out, err := command.CombinedOutput()
	t.Logf("%s output:\n%s", command, out)
	if err != nil {
		t.Fatalf("exec.CommandContext(%s).CombinedOutput() failed unexpectedly; err = %v", command, err)
	}
}

// Exercise the RDMA stack using https://github.com/linux-rdma/perftest. This is a performance test.
func TestIBWriteBWHost(t *testing.T) {
	ctx := utils.Context(t)
	acceleratorutils.InstallIbVerbsUtils(ctx, t)
	acceleratorutils.SetupRDMAPerftestLibrary(ctx, t)
	nics := findRDMANICs(ctx, t)
	for _, nic := range nics {
		// Run a warmup iteration for better test performance and consistency, as ib_write_bw's built-in
		// warmup does not provide consistent results.
		runIbWriteBWHost(ctx, t, nic)

		// Run the command again for the actual performance test.
		runIbWriteBWHost(ctx, t, nic)
	}
}

// Exercise the RDMA stack using https://github.com/linux-rdma/perftest. This is a performance test.
func TestIBWriteBWClient(t *testing.T) {
	ctx := utils.Context(t)
	acceleratorutils.InstallIbVerbsUtils(ctx, t)
	acceleratorutils.SetupRDMAPerftestLibrary(ctx, t)
	nics := findRDMANICs(ctx, t)

	for _, nic := range nics {
		args := append(ibWriteBWArgs, fmt.Sprintf("--ib-dev=%s", nic))
		// Run a warmup iteration for better test performance and consistency, as ib_write_bw's built-in
		// warmup does not provide consistent results.
		acceleratorutils.RunRDMAClientCommand(ctx, t, "./ib_write_bw", args)

		// Run the command again for the actual test to confirm bandwidth is above the threshold.
		out := acceleratorutils.RunRDMAClientCommand(ctx, t, "./ib_write_bw", args)
		bandwidth := extractPerfTestAverageBandwidth(t, out)
		t.Logf("Average bandwidth for device %s: %.2f (gbps)", nic, bandwidth)
		if bandwidth < expectedMinBandwidth {
			t.Errorf("Average bandwidth result for device %s is %f (gbps), which is below the expected threshold of %.2f (gbps) (80%% of line rate %d gbps)", nic, bandwidth, expectedMinBandwidth, a3uAndA4LineRate)
		}
	}
}

const (
	rdmaUbuntuNICPrefix  = "roce"
	rdmaRockyNICPrefix   = "mlx5"
	expectedRDMANICCount = 8 // A3U/A4 VMs have 8 RDMA NICs.
)

func findRDMANICs(ctx context.Context, t *testing.T) []string {
	t.Helper()
	out, err := exec.CommandContext(ctx, "ibv_devinfo", "--list").CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, ibv_devinfo --list).CombinedOutput() = failed unexpectedly; err = %v\noutput: %s", err, out)
	}
	nics := make([]string, 0, expectedRDMANICCount)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, rdmaUbuntuNICPrefix) || strings.HasPrefix(line, rdmaRockyNICPrefix) {
			nics = append(nics, line)
		}
	}
	if len(nics) != expectedRDMANICCount {
		t.Fatalf("Expected 8 RDMA NICs for A3U/A4 VMs, found %d, NICs found: %v", len(nics), nics)
	}
	return nics
}

// The expected output columns are 'msg_size', 'iterations', 'peak_gbps', 'avg_gbps' and,
// 'msg_rate_mpps'.
const (
	ibWriteBandwidthColumnsLen = 5
	averageBandwidthIndex      = 3
)

// extractPerfTestAverageBandwidth extracts the average bandwidth result from ib_write_bw test
// output. It returns the first result it finds.
func extractPerfTestAverageBandwidth(t *testing.T, outputString string) float64 {
	t.Helper()
	// Matches lines that only contain whitespace, digits, and periods.
	regex := regexp.MustCompile(`^[\s\d.]+$`)
	for _, line := range strings.Split(outputString, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Skip lines containing chars other than whitespace, numbers, or '.'.
		if !regex.MatchString(line) {
			continue
		}
		values := strings.Fields(line)
		if len(values) != ibWriteBandwidthColumnsLen {
			t.Logf(
				"Line %q has an unexpected number of columns (got %d, expected %d)",
				line,
				len(values),
				ibWriteBandwidthColumnsLen,
			)
			continue
		}
		averageBWStr := values[averageBandwidthIndex]
		averageBW, err := strconv.ParseFloat(averageBWStr, 64)
		if err != nil {
			t.Logf("Failed to parse average bandwidth %q: %v", averageBWStr, err)
			continue
		}
		return averageBW
	}
	t.Fatalf("No average bandwidth result found in output:\n%s", outputString)
	return -1
}
