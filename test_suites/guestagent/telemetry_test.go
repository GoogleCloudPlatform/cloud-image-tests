// Copyright 2023 Google LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     https://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package guestagent

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func getAgentOutput(t *testing.T) string {
	t.Helper()
	if utils.IsWindows() {
		out, err := utils.RunPowershellCmd(`(Get-WinEvent -Providername GCEGuestAgentManager).Message`)
		if err != nil {
			t.Fatalf("could not get agent output: %v", err)
		}
		return string(out.Stdout)
	}
	out, err := exec.CommandContext(utils.Context(t), "journalctl", "-o", "cat", "-eu", "google-guest-agent-manager").Output()
	if err != nil {
		t.Fatalf("could not get agent output: %v", err)
	}
	return string(out)
}

func TestTelemetryEnabled(t *testing.T) {
	time.Sleep(time.Second)
	totaloutput := getAgentOutput(t)
	if !strings.Contains(totaloutput, "telemetry") {
		t.Skip("agent does not support telemetry")
	}
	if strings.Contains(totaloutput, "Successfully scheduled job telemetryJobID") {
		// Scheduled by non-core plugin agent.
		return
	}
	if strings.Contains(totaloutput, "Failed module: telemetry-publisher") {
		t.Errorf("Telemetry jobs are scheduled after setting disable-guest-telemetry=true. Agent logs: %s", totaloutput)
	}
}

func TestTelemetryDisabled(t *testing.T) {
	time.Sleep(time.Second)
	initialoutput := getAgentOutput(t)
	if !strings.Contains(initialoutput, "telemetry") {
		t.Skip("agent does not support telemetry")
	}
	if strings.Contains(initialoutput, "Failed to schedule job telemetryJobID") {
		// Scheduled by non-core plugin agent.
		return
	}
	if !strings.Contains(initialoutput, "Failed module: telemetry-publisher") {
		t.Errorf("Telemetry jobs are scheduled after setting disable-guest-telemetry=true. Agent logs: %s", initialoutput)
	}
}
