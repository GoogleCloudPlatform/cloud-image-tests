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

package packagemanager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

var (
	repoAvailabilityTestCmdArgs = map[string][]string{
		"apt":    {"-y", "update"},
		"yum":    {"-y", "makecache"},
		"dnf":    {"-y", "makecache"},
		"zypper": {"refresh"},
		"googet": {"available"},
	}
	googetErrorMatch = "ERROR:"
)

func TestRepoReachabilityDualStack(t *testing.T) {
	testRepoAvailability(t)
}

func TestRepoReachabilityIPv4Only(t *testing.T) {
	testRepoAvailability(t)
}

func testRepoAvailability(t *testing.T) {
	ctx := utils.Context(t)
	for pm, args := range repoAvailabilityTestCmdArgs {
		if !utils.CheckLinuxCmdExists(pm) {
			continue
		}
		cmd := exec.CommandContext(ctx, pm, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("exec.CommandContext(%s).CombinedOutput() = output: %s err: %v, want err: nil", cmd.String(), out, err)
		}
		if pm == "googet" && strings.Contains(string(out), googetErrorMatch) {
			// Googet does not set failing exit code when repos are unreachable.
			t.Fatalf("exec.CommandContext(%s).CombinedOutput() = output: %s which contains %q", cmd.String(), out, googetErrorMatch)
		}
	}
}

func TestRemoveAgentSetup(t *testing.T) {
	markerFile := filepath.Join(os.TempDir(), "marker")
	if utils.Exists(markerFile, utils.TypeFile) {
		pid, err := os.ReadFile(markerFile)
		if err != nil {
			t.Fatalf("os.ReadFile(%s) = %v, want nil", markerFile, err)
		}

		pids := strings.Split(strings.TrimSpace(string(pid)), "--")
		t.Logf("Read Pids from marker file: %v", pids)

		pidInt, err := strconv.Atoi(pids[0])
		if err != nil {
			t.Fatalf("Error converting string %q to int: %v", pids[0], err)
		}
		parentPidInt, err := strconv.Atoi(pids[1])
		if err != nil {
			t.Fatalf("Error converting string %q to int: %v", pids[1], err)
		}

		// Kill parent process first as it would end up returning/uploading results
		// for the wrong test.
		killProcess(t, parentPidInt)
		killProcess(t, pidInt)

		removeAgent(t)
		validateAgentRemoved(t)
	} else {
		// setup for test
		currentPid := os.Getpid()
		parentPid := os.Getppid()
		write := fmt.Sprintf("%d--%d", currentPid, parentPid)
		err := os.WriteFile(markerFile, []byte(write), 0644)
		if err != nil {
			t.Fatalf("os.Create(%s) = %v, want nil", markerFile, err)
		}
		t.Logf("Created marker file: %q, with current pid: %d, parent pid: %d", markerFile, currentPid, parentPid)

		configurePackageRemoveTask(t)
		// Package removal test should kill this process. Until then, wait to avoid
		// race conditions as it would end up returning results for the wrong test.
		time.Sleep(1 * time.Hour)
	}
}
