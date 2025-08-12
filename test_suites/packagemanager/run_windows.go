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
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

var scheduleTaskCmd = `
$TaskName = "TestPackageRemoveTask"
$ExecutablePath = "C:\Program Files\Google\Compute Engine\GCEMetadataScripts.exe"
$TaskDescription = "A task created by CIT test to validate package removal."

# Set a start time 3000 seconds in the future. We don't need it to run as we're 
# explicitly triggering it.
$startTime = (Get-Date).AddSeconds(3000)
$formattedTime = $startTime.ToString('HH:mm:ss')

# Action to be performed by the task.
$action = New-ScheduledTaskAction -Execute $ExecutablePath -Argument startup

# Trigger for the task.
$trigger = New-ScheduledTaskTrigger -Once -At $formattedTime

# Register the task with the task scheduler.
Register-ScheduledTask -TaskName $TaskName -Description $TaskDescription -Trigger $trigger -Action $action -User "NT AUTHORITY\SYSTEM"

# Manually trigger the task to run immediately.
Start-ScheduledTask -TaskName $TaskName

Write-Host "Scheduled task '$TaskName' has been created and triggered to run."
`

func configurePackageRemoveTask(t *testing.T) {
	t.Helper()
	if err := utils.CopyFile(`C:\Program Files\Google\Compute Engine\metadata_scripts\GCEMetadataScripts.exe`, `C:\Program Files\Google\Compute Engine\GCEMetadataScripts.exe`); err != nil {
		t.Fatalf("Failed to copy create a copy ofGCEMetadataScripts.exe to = %v, want nil", err)
	}

	out, err := utils.RunPowershellCmd(scheduleTaskCmd)
	if err != nil {
		t.Fatalf("Failed to schedule task, output= %+v, error: %+v", out, err)
	}
}

func validateAgentRemoved(t *testing.T) {
	t.Helper()
	agentDir := `C:\Program Files\Google\Compute Engine\agent`
	files := []string{"GCEWindowsAgent.exe", "GCEWindowsAgentManager.exe", "CorePlugin.exe"}
	for _, file := range files {
		agentFile := filepath.Join(agentDir, file)
		if utils.Exists(agentFile, utils.TypeFile) {
			t.Fatalf("Install file %q still exists", agentFile)
		}
	}
	utils.ProcessExistsWindows(t, false, "CorePlugin")
	t.Logf("Successfully validated google-compute-engine-windows has been removed.")
}

func removeAgent(t *testing.T) {
	t.Helper()
	stopStartup := `Stop-ScheduledTask -TaskName "GCEStartup"`
	out, err := utils.RunPowershellCmd(stopStartup)
	if err != nil {
		t.Fatalf("Failed to stop startup script, output:= %+v, error: %+v", out, err)
	}

	cmd := exec.Command("googet", "-noconfirm", "remove", "google-compute-engine-windows")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("googet remove google-compute-engine-windows failed: %v, output: %s", err, string(out))
	}
	t.Logf("Successfully removed google-compute-engine-windows")
}

func killProcess(t *testing.T, pid int) {
	t.Helper()
	cmd := exec.Command("taskkill", "/F", "/PID", fmt.Sprintf("%d", pid))
	if err := cmd.Run(); err != nil {
		t.Logf("taskkill -9 %d failed: %v", pid, err)
	} else {
		t.Logf("Killed process successfully with pid: %d", pid)
	}
}
