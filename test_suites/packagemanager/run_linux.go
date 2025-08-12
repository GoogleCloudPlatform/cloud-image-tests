// Copyright 2025 Google LLC.
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
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func configurePackageRemoveTask(t *testing.T) {
	t.Helper()
	ctx := utils.Context(t)
	cfg := `
[Unit]
Description=Google Compute Engine Startup Scripts
Wants=network-online.target rsyslog.service
After=network-online.target rsyslog.service google-guest-agent-manager.service
Before=apt-daily.service

[Service]
Type=oneshot
ExecStart=/usr/bin/google_metadata_script_runner_adapt startup
#TimeoutStartSec is ignored for Type=oneshot service units.
KillMode=process
StandardOutput=journal+console
StandardError=journal+console

[Install]
WantedBy=multi-user.target
`
	if err := os.WriteFile("/etc/systemd/system/google-test-scripts.service", []byte(cfg), 0644); err != nil {
		t.Fatalf("os.WriteFile(%s) = %v, want nil", "/etc/systemd/system/google-test-scripts.service", err)
	}

	reloadCmd := exec.CommandContext(ctx, "systemctl", "daemon-reload")
	if err := reloadCmd.Run(); err != nil {
		t.Fatalf("systemctl daemon-reload failed: %v", err)
	}
	startCmd := exec.CommandContext(ctx, "systemctl", "start", "google-test-scripts.service")
	if err := startCmd.Run(); err != nil {
		t.Fatalf("systemctl start google-test-scripts.service failed: %v", err)
	}
}

func validateAgentRemoved(t *testing.T) {
	t.Helper()
	agentFiles := []string{"/usr/bin/google_guest_agent", "/usr/bin/google_guest_agent_manager", "/usr/lib/google/guest_agent/core_plugin"}
	for _, file := range agentFiles {
		if utils.Exists(file, utils.TypeFile) {
			t.Fatalf("Install file %q still exists", file)
		}
	}
	utils.ProcessExistsLinux(t, false, "/usr/lib/google/guest_agent/core_plugin")
	t.Logf("Successfully validated google-guest-agent has been removed.")
}

var removeAgentCmdArgs = map[string][]string{
	"apt-get": {"-y", "remove", "google-guest-agent"},
	"dnf":     {"-y", "remove", "google-guest-agent"},
	"yum":     {"-y", "remove", "google-guest-agent"},
	"zypper":  {"--non-interactive", "remove", "google-guest-agent"},
}

func removeAgent(t *testing.T) {
	ctx := utils.Context(t)
	var ran bool
	for pm, args := range removeAgentCmdArgs {
		if !utils.CheckLinuxCmdExists(pm) {
			continue
		}

		ran = true
		cmd := exec.CommandContext(ctx, pm, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("exec.CommandContext(%s).CombinedOutput() = output: %s err: %v, want err: nil", cmd.String(), string(out), err)
		}
		break
	}
	if !ran {
		t.Fatalf("No known package manager found to remove google-guest-agent")
	}
	t.Logf("Successfully removed google-guest-agent")
}

func killProcess(t *testing.T, pid int) {
	t.Helper()
	cmd := exec.Command("kill", "-9", fmt.Sprintf("%d", pid))
	if err := cmd.Run(); err != nil {
		t.Logf("kill -9 %d failed: %v", pid, err)
	} else {
		t.Logf("Killed process successfully with pid: %d", pid)
	}
}
