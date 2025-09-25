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

package acceleratorrdma

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

var pingPongArgs = []string{
	"--gid-idx=3",
}

func installIbvUtils(ctx context.Context, t *testing.T) {
	t.Helper()
	// Rocky Linux has ibv_rc_pingpong pre-installed.
	if isRockyLinux(ctx, t) {
		return
	}
	if out, err := exec.CommandContext(ctx, "apt", "update").CombinedOutput(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, apt update).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", err, out)
	}
	if out, err := exec.CommandContext(ctx, "apt", "install", "-y", "ibverbs-utils").CombinedOutput(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, apt install, -y, ibverbs-utils).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", err, out)
	}
}

func isRockyLinux(ctx context.Context, t *testing.T) bool {
	t.Helper()
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Logf("Could not read /etc/os-release: %v, defaulting isRockyLinux to false", err)
		return false
	}
	return strings.Contains(string(content), "rocky")
}

func TestRDMAPingPongHost(t *testing.T) {
	ctx := utils.Context(t)
	installIbvUtils(ctx, t)
	pingPongCmd := exec.CommandContext(ctx, "ibv_rc_pingpong", pingPongArgs...)
	out, err := pingPongCmd.CombinedOutput()
	t.Logf("%s output:\n%s", pingPongCmd, out)
	if err != nil {
		t.Fatalf("exec.CommandContext(%q).CombinedOutput() failed unexpectedly; err = %v", pingPongCmd, err)
	}
}

func TestRDMAPingPongClient(t *testing.T) {
	ctx := utils.Context(t)
	installIbvUtils(ctx, t)
	runRDMAClientCommand(ctx, t, "ibv_rc_pingpong", pingPongArgs)
}
