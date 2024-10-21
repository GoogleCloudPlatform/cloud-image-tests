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
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func restartAgent(ctx context.Context, t *testing.T) {
	t.Helper()
	var cmd *exec.Cmd
	if utils.IsWindows() {
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NonInteractive", "Restart-Service", "GCEAgent")
	} else {
		cmd = exec.CommandContext(ctx, "systemctl", "restart", "google-guest-agent")
	}
	err := cmd.Run()
	if err != nil {
		t.Fatalf("could not restart agent: %v", err)
	}
}

func getAgentOutput(ctx context.Context, t *testing.T) string {
	t.Helper()
	if utils.IsWindows() {
		out, err := utils.RunPowershellCmd(`(Get-WinEvent -Providername GCEGuestAgent).Message`)
		if err != nil {
			t.Fatalf("could not get agent output: %v", err)
		}
		return string(out.Stdout)
	}
	out, err := exec.CommandContext(ctx, "journalctl", "-o", "cat", "-eu", "google-guest-agent").Output()
	if err != nil {
		t.Fatalf("could not get agent output: %v", err)
	}
	return string(out)
}

func TestTelemetryEnabled(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	initialoutput := getAgentOutput(ctx, t)
	client, err := utils.GetDaisyClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prj, zone, err := utils.GetProjectZone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	name, err := utils.GetMetadata(ctx, "instance", "name")
	if err != nil {
		t.Fatal(err)
	}
	inst, err := client.GetInstance(prj, zone, name)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inst.Metadata.Items {
		if item.Key == "disable-guest-telemetry" {
			s := "true"
			item.Value = &s
		}
	}
	err = client.SetInstanceMetadata(prj, zone, name, inst.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	restartAgent(ctx, t)
	time.Sleep(time.Second)
	totaloutput := getAgentOutput(ctx, t)
	finaloutput := strings.TrimPrefix(totaloutput, initialoutput)
	if !strings.Contains(totaloutput, "telemetry") {
		t.Skip("agent does not support telemetry")
	}
	if !strings.Contains(initialoutput, "Successfully scheduled job telemetryJobID") {
		t.Errorf("Telemetry jobs are not scheduled by default. Agent logs: %s", initialoutput)
	}
	if !strings.Contains(finaloutput, "Failed to schedule job telemetryJobID") {
		t.Errorf("Telemetry jobs are scheduled after setting disable-guest-telemetry=true. Agent logs: %s", finaloutput)
	}
}

func TestTelemetryDisabled(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	initialoutput := getAgentOutput(ctx, t)
	client, err := utils.GetDaisyClient(ctx)
	if err != nil {
		t.Fatal(err)
	}
	prj, zone, err := utils.GetProjectZone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	name, err := utils.GetMetadata(ctx, "instance", "name")
	if err != nil {
		t.Fatal(err)
	}
	inst, err := client.GetInstance(prj, zone, name)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inst.Metadata.Items {
		if item.Key == "disable-guest-telemetry" {
			s := "false"
			item.Value = &s
		}
	}
	err = client.SetInstanceMetadata(prj, zone, name, inst.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	restartAgent(ctx, t)
	time.Sleep(time.Second)
	totaloutput := getAgentOutput(ctx, t)
	finaloutput := strings.TrimPrefix(totaloutput, initialoutput)
	if !strings.Contains(totaloutput, "telemetry") {
		t.Skip("agent does not support telemetry")
	}
	if !strings.Contains(initialoutput, "Failed to schedule job telemetryJobID") {
		t.Errorf("Telemetry jobs are scheduled after setting disable-guest-telemetry=true. Agent logs: %s", initialoutput)
	}
	if !strings.Contains(finaloutput, "Successfully scheduled job telemetryJobID") {
		t.Errorf("Telemetry jobs are not scheduled after setting disable-guest-telemetry=false. Agent logs: %s", finaloutput)
	}
}
