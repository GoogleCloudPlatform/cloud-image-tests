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
	"os/exec"
	"strings"
	"testing"

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
	ctx, cancel := utils.Context(t)
	defer cancel()
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
