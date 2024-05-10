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

package metadata

import (
	"context"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func guestAgentPackageName() string {
	if utils.IsWindows() {
		return "google-compute-engine-windows"
	}
	return "google-guest-agent"
}

func reinstallGuestAgent(ctx context.Context, t *testing.T) {
	t.Helper()
	pkg := guestAgentPackageName()
	if utils.IsWindows() {
		cmd := exec.CommandContext(ctx, "googet", "install", "-reinstall", pkg)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
		// Respond to "Reinstall pkg? (y/N):" prompt
		io.WriteString(stdin, "y\r\n")
		if err := cmd.Wait(); err != nil {
			t.Fatalf("Failed waiting to reinstall agent: %v", err)
		}
		return
	}
	var cmd, fallback, prep *exec.Cmd
	switch {
	case utils.CheckLinuxCmdExists("apt"):
		prep = exec.CommandContext(ctx, "apt", "update", "-y")
		cmd = exec.CommandContext(ctx, "apt", "reinstall", "-y", pkg)
		fallback = exec.CommandContext(ctx, "apt", "install", "-y", "--reinstall", pkg)
	case utils.CheckLinuxCmdExists("dnf"):
		repoArg := "--repo=google-compute-engine"
		cmdTokens := []string{"dnf", "-y", "reinstall", pkg, repoArg}
		cmd = exec.CommandContext(ctx, cmdTokens[0], cmdTokens[1:]...)

		cmdTokens = []string{"dnf", "-y", "upgrade", pkg, repoArg}
		fallback = exec.CommandContext(ctx, cmdTokens[0], cmdTokens[1:]...)
	case utils.CheckLinuxCmdExists("yum"):
		repoArgs := []string{"--disablerepo='*'", "--enablerepo=google-compute-engine"}
		cmdTokens := []string{"yum", "-y", "reinstall", pkg}
		cmdTokens = append(cmdTokens, repoArgs...)
		cmd = exec.CommandContext(ctx, cmdTokens[0], cmdTokens[1:]...)

		cmdTokens = []string{"yum", "-y", "upgrade", pkg}
		cmdTokens = append(cmdTokens, repoArgs...)
		fallback = exec.CommandContext(ctx, cmdTokens[0], cmdTokens[1:]...)
	case utils.CheckLinuxCmdExists("zypper"):
		cmd = exec.CommandContext(ctx, "zypper", "--non-interactive", "install", "--force", pkg)
		fallback = exec.CommandContext(ctx, "zypper", "--non-interactive", "install", "--force", pkg)
		fallback.Env = append(fallback.Env, "ZYPP_LOCK_TIMEOUT=5184000") // A negative value is supposed to wait forever but older versions of libzypp are bugged. This will wait for 24 hours.
	default:
		t.Fatalf("could not find a package manager to reinstall %s with", pkg)
		return
	}
	if prep != nil {
		if err := prep.Run(); err != nil {
			t.Logf("could not prep to reinstall %s: %v", pkg, err)
		}
	}

	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		if fallback != nil {
			fallbackOutput, err := fallback.CombinedOutput()
			if err != nil {
				t.Fatalf("could not reinstall %s with fallback: %s, output: %s",
					pkg, err, string(fallbackOutput))
			}
		} else {
			t.Fatalf("could not reinstall %s: %s, output: %s", pkg, err, string(cmdOutput))
		}
	}
}
