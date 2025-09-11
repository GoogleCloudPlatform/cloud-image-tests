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
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func isRockyLinux(ctx context.Context, t *testing.T) bool {
	t.Helper()
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Logf("Could not read os-release: %v, defaulting isRockyLinux to false", err)
		return false
	}
	return strings.Contains(string(content), "rocky")
}

func installShowGids(ctx context.Context, t *testing.T) {
	t.Helper()
	// Rocky Linux accelerator OS already contains show_gids
	if isRockyLinux(ctx, t) {
		return
	}
	if err := utils.InstallPackage("git"); err != nil {
		t.Fatalf("Failed to install git, err = %v", err)
	}
	if out, err := exec.CommandContext(ctx, "git", "clone", "--depth=1", "https://github.com/Mellanox/mlnx-tools.git").CombinedOutput(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, git clone https://github.com/Mellanox/mlnx-tools.git).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", err, out)
	}
}

func runShowGids(ctx context.Context, t *testing.T) string {
	t.Helper()
	command := "./mlnx-tools/sbin/show_gids"
	if isRockyLinux(ctx, t) {
		command = "show_gids"
	}
	out, err := exec.CommandContext(ctx, command).CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, %s).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", command, err, out)
	}
	t.Logf("%s output:\n%s", command, out)
	return string(out)
}

func validateGIDTable(t *testing.T, gidTable string) {
	t.Helper()
	if !strings.Contains(gidTable, "n_gids_found=32") {
		t.Fatalf("The gid table does not contain `n_gids_found=32`\nGID table: %q", gidTable)
	}

	// A3U/A4 VMs have 8 RDMA NICs. Each NIC should have 4 GID entries with indexes 0, 1, 2, and 3.
	indexCounts := make(map[int64]int)
	for _, line := range strings.Split(gidTable, "\n") {
		row := strings.Fields(line)
		if len(row) < 2 {
			continue
		}
		index, err := strconv.ParseInt(row[2], 10, 64)
		if err != nil {
			continue
		}
		if index < 0 || index > 3 {
			t.Errorf("gid index %d is out of the range [0, 3]", index)
			continue
		}
		indexCounts[index]++
	}

	for index, count := range indexCounts {
		if count != 8 {
			t.Errorf("Wanted 8 GID entries for index %d, got %d", index, count)
		}
	}
}

func resetGIDTable(ctx context.Context, t *testing.T) {
	t.Helper()
	service := "systemd-networkd.service"
	if isRockyLinux(ctx, t) {
		service = "NetworkManager.service"
	}
	t.Logf("Restarting %s to trigger a GID table rebuild", service)
	command := exec.CommandContext(ctx, "systemctl", "restart", service)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, %s).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", command, err, out)
	}

	// Wait a maximum of 10 seconds for the GID table to be rebuilt.
	ts := time.Now()
	for time.Since(ts) < time.Second*10 {
		gidTable := runShowGids(ctx, t)
		if strings.Contains(gidTable, "n_gids_found=32") {
			return
		}
		time.Sleep(time.Millisecond * 100)
	}
	t.Log("GID table is not succesfully rebuilt after 10 seconds. Continuing the test anyways.")
}

func TestGids(t *testing.T) {
	ctx := utils.Context(t)
	installShowGids(ctx, t)
	gidTable := runShowGids(ctx, t)
	validateGIDTable(t, gidTable)

	for i := 0; i < 3; i++ {
		resetGIDTable(ctx, t)
		runShowGids(ctx, t)
		validateGIDTable(t, gidTable)
	}
}
