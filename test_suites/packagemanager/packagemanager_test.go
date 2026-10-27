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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const windowsGooget = `C:\ProgramData\GooGet\googet.exe`

var (
	repoAvailabilityTestCmdArgs = map[string][]string{
		"apt":    {"-y", "update"},
		"yum":    {"-y", "makecache"},
		"dnf":    {"-y", "makecache"},
		"zypper": {"refresh"},
		"googet": {"available"},
	}
	repoListCmdArgs = map[string][]string{
		"apt":    {"policy"},
		"yum":    {"repolist", "-v"},
		"dnf":    {"repolist", "-v"},
		"googet": {"listrepos"},
	}
	googetErrorMatch   = "ERROR:"
	nonStableRepoRegex = regexp.MustCompile(`google-compute-engine-(?:[^\s/,]+-)?(canary|global|staging|testing|unstable)\b`)
	stableRepoRegex    = regexp.MustCompile(`google-compute-engine-(?:[^\s/,]+-)?stable\b`)
)

func TestRepoReachabilityDualStack(t *testing.T) {
	testRepoAvailability(t)
}

func TestRepoReachabilityIPv4Only(t *testing.T) {
	testRepoAvailability(t)
}

func cmdExists(pm string) (string, bool) {
	if utils.IsWindows() {
		if pm != "googet" {
			return "", false
		}
		if path, err := exec.LookPath(pm); err == nil {
			return path, true
		}
		if utils.Exists(windowsGooget, utils.TypeFile) {
			return windowsGooget, true
		}
		return "", false
	}
	if utils.CheckLinuxCmdExists(pm) {
		return pm, true
	}
	return "", false
}

func validateRepoConfig(ctx context.Context, t *testing.T, pm, cmdPath, image string) {
	t.Helper()
	args, ok := repoListCmdArgs[pm]
	if !ok {
		return
	}
	cmd := exec.CommandContext(ctx, cmdPath, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(%s).CombinedOutput() = output: %s err: %v, want err: nil", cmd.String(), out, err)
	}
	output := string(out)
	if match := nonStableRepoRegex.FindString(output); match != "" {
		t.Fatalf("%s enabled repos reference a non-stable google-compute-engine repo %q; output:\n%s", pm, match, output)
	}
	requireStableRepo := utils.IsDebian(image) || utils.IsRHEL(image) || utils.IsCentOS(image) || utils.IsRocky(image) || utils.IsAlmaLinux(image) || utils.IsOracle(image) || utils.IsWindows()
	if requireStableRepo && !stableRepoRegex.MatchString(output) {
		t.Fatalf("%s enabled repos do not include a google-compute-engine stable repo; output:\n%s", pm, output)
	}
}

func testRepoAvailability(t *testing.T) {
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("utils.GetMetadata(ctx, instance, image) = %v, want nil", err)
	}
	for pm, args := range repoAvailabilityTestCmdArgs {
		cmdPath, ok := cmdExists(pm)
		if !ok {
			continue
		}
		cmd := exec.CommandContext(ctx, cmdPath, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("exec.CommandContext(%s).CombinedOutput() = output: %s err: %v, want err: nil", cmd.String(), out, err)
		}
		if pm == "googet" && strings.Contains(string(out), googetErrorMatch) {
			// Googet does not set failing exit code when repos are unreachable.
			t.Fatalf("exec.CommandContext(%s).CombinedOutput() = output: %s which contains %q", cmd.String(), out, googetErrorMatch)
		}
		validateRepoConfig(ctx, t, pm, cmdPath, image)
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
