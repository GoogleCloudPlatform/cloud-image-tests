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

package compatmanager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func isCorePluginCfgEnabled(t *testing.T) bool {
	t.Helper()
	path := "/etc/google-guest-agent/core-plugin-enabled"
	if utils.IsWindows() {
		path = `C:\ProgramData\Google\Compute Engine\google-guest-agent\core-plugin-enabled`
	}

	if !utils.Exists(path, utils.TypeFile) {
		return false
	}

	exp := regexp.MustCompile(`enabled=(\w+)`)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file %q: %v", path, err)
	}

	matches := exp.FindStringSubmatch(string(data))
	if len(matches) != 2 {
		t.Fatalf("Failed to parse config file %q, unknown format (%q): %v", path, string(data), err)
	}

	corePluginEnabled, err := strconv.ParseBool(matches[1])
	if err != nil {
		t.Fatalf("Failed to parse bool (%q) value from config file %q: %v", matches[1], path, err)
	}

	return corePluginEnabled
}

func checkAgentManagerIsRunning(t *testing.T) {
	t.Helper()
	// Manager process should always be running.
	if utils.IsWindows() {
		utils.ProcessExistsWindows(t, true, "GCEWindowsAgentManager")
	} else {
		utils.ProcessExistsLinux(t, true, "/usr/bin/google_guest_agent_manager")
	}
}

func checkGuestAgentIsRunning(t *testing.T, wantRunning bool) {
	t.Helper()

	if utils.IsWindows() {
		utils.ProcessExistsWindows(t, wantRunning, "GCEWindowsAgent")
	} else {
		utils.ProcessExistsLinux(t, wantRunning, "/usr/bin/google_guest_agent")
	}
}

func serviceEnabledLinux(t *testing.T, wantEnabled bool, serviceName string) {
	t.Helper()
	cmd := exec.Command("systemctl", "is-enabled", serviceName)
	out, err := cmd.CombinedOutput()
	// systemctl is-enabled returns 1 if it is disabled. Don't check only for err
	// returned.
	output := strings.TrimSpace(string(out))
	if got := (output == "enabled"); got != wantEnabled {
		t.Fatalf("Service %q expected to be enabled: [%t], got: [%t]\nOutput:\n[%q]\nError: [%v]", serviceName, wantEnabled, got, output, err)
	}
}

func serviceEnabledWindows(t *testing.T, wantEnabled bool, serviceName string) {
	t.Helper()

	cmd := fmt.Sprintf("Get-Service -Name %s | Select-Object Name, StartType", serviceName)
	output, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		t.Fatalf("Failed to run powershell command %q: %v", cmd, err)
	}

	if got := strings.Contains(output.Stdout, "Automatic"); got != wantEnabled {
		t.Fatalf("Service %q expected to be enabled: %t, got: %t\nOutput:\n %s\nStderr: %s", serviceName, wantEnabled, got, output.Stdout, output.Stderr)
	}
}

func checkGuestAgentIsEnabled(t *testing.T, wantRunning bool) {
	t.Helper()

	if utils.IsWindows() {
		serviceEnabledWindows(t, wantRunning, "GCEAgent")
	} else {
		serviceEnabledLinux(t, wantRunning, "google-guest-agent")
	}
}

func checkCorePluginProcessExists(t *testing.T, exists bool) {
	t.Helper()

	if utils.IsWindows() {
		utils.ProcessExistsWindows(t, exists, "CorePlugin")
	} else {
		utils.ProcessExistsLinux(t, exists, "/usr/lib/google/guest_agent/core_plugin")
	}
}

func skipIfNoCompatManager(t *testing.T) {
	filePath := "/usr/bin/google_guest_compat_manager"
	if utils.IsWindows() {
		filePath = `C:\Program Files\Google\Compute Engine\agent\GCEWindowsCompatManager.exe`
	}
	if !utils.Exists(filePath, utils.TypeFile) {
		t.Skip("Compat Manager is not installed on the image, skipping the test.")
	}
}

func checkCompatManagerIsDisabled(t *testing.T) {
	t.Helper()

	if utils.IsWindows() {
		serviceEnabledWindows(t, false, "GCEWindowsCompatManager")
	} else {
		serviceEnabledLinux(t, false, "google-guest-compat-manager")
	}
}

func TestCompatManager(t *testing.T) {
	ctx := context.Background()
	skipIfNoCompatManager(t)

	// It could be rollforward package or old agent package.
	if utils.IsCoreDisabled() {
		if !utils.IfOldAgentInstalled() {
			checkCompatManagerIsDisabled(t)
		}
		t.Logf("Core plugin is disabled, skipping remaining compat manager tests.")
		return
	}

	tests := []struct {
		name                  string
		setMetadata           string
		wantCorePluginEnabled bool
		wantAgentRunning      bool
		shouldFileExist       bool
	}{
		{
			name:                  "default",
			wantCorePluginEnabled: true,
			wantAgentRunning:      false,
			shouldFileExist:       false,
		},
		{
			name:                  "disable_core_plugin",
			setMetadata:           "false",
			wantCorePluginEnabled: false,
			wantAgentRunning:      true,
			shouldFileExist:       true,
		},
		{
			name:                  "enable_core_plugin",
			setMetadata:           "true",
			wantCorePluginEnabled: true,
			wantAgentRunning:      false,
			shouldFileExist:       true,
		},
	}

	// Tests must be run sequentially to validate the expected behavior of the
	// core plugin.
	for _, tc := range tests {
		t.Logf("Running test: %s", tc.name)

		if tc.setMetadata != "" {
			if err := utils.UpsertMetadata(ctx, "enable-guest-agent-core-plugin", tc.setMetadata); err != nil {
				t.Fatalf("Failed to enable guest agent core plugin: %v", err)
			}
		}

		// Watcher is monitoring MDS changes, wait for some time for the watcher to
		// pick up the change.
		if tc.shouldFileExist {
			conditionMet := false
			var lastCfgEnabled bool
			for i := 0; i < 10; i++ {
				lastCfgEnabled = isCorePluginCfgEnabled(t)
				if lastCfgEnabled == tc.wantCorePluginEnabled {
					conditionMet = true
					break
				}
				time.Sleep(time.Duration(i*2) * time.Second)
			}

			if !conditionMet {
				t.Fatalf("Core plugin enabled in config file is [%t], want [%t] after setting metadata.", lastCfgEnabled, tc.wantCorePluginEnabled)
			}
		}

		// Wait for manager to install/uninstall the core plugin.
		time.Sleep(time.Second * 90)

		checkAgentManagerIsRunning(t)
		checkGuestAgentIsEnabled(t, tc.wantAgentRunning)
		checkCorePluginProcessExists(t, tc.wantCorePluginEnabled)
		checkGuestAgentIsRunning(t, tc.wantAgentRunning)
		// Add some delay between tests to let processes run for a while.
		time.Sleep(time.Second * 30)
	}
}
