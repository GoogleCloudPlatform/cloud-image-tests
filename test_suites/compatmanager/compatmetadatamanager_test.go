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

package compatmanager

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func skipIfNoMetadataScriptCompat(t *testing.T) {
	filePath := "/usr/bin/gce_compat_metadata_script_runner"
	if utils.IsWindows() {
		filePath = `C:\Program Files\Google\Compute Engine\metadata_scripts\GCECompatMetadataScripts.exe`
	}
	if !utils.Exists(filePath, utils.TypeFile) {
		t.Skipf("Metadata script compat manager %q is not installed on the image, skipping the test.", filePath)
	}
}

func getProcessLists(corePluginEnabled bool) (wantProcesses, dontWantProcess []string) {
	if utils.IsWindows() {
		if corePluginEnabled {
			return []string{"GCECompatMetadataScripts", "GCEMetadataScriptRunner"}, []string{"GCEMetadataScripts"}
		}
		return []string{"GCECompatMetadataScripts", "GCEMetadataScripts"}, []string{"GCEMetadataScriptRunner"}
	}

	if corePluginEnabled {
		return []string{"/usr/bin/gce_compat_metadata_script_runner", "/usr/bin/gce_metadata_script_runner"}, []string{"/usr/bin/google_metadata_script_runner"}
	}
	return []string{"/usr/bin/gce_compat_metadata_script_runner", "/usr/bin/google_metadata_script_runner"}, []string{"/usr/bin/gce_metadata_script_runner"}
}

func processExists(t *testing.T, shouldExist bool, processName string) {
	if utils.IsWindows() {
		processExistsWindows(t, shouldExist, processName)
	} else {
		processExistsLinux(t, shouldExist, processName)
	}
}

func verifyMetadataScriptsProcesses(t *testing.T, corePluginEnabled bool) {
	t.Helper()
	wantedProcesses, dontWantProcesses := getProcessLists(corePluginEnabled)

	for _, wantProcess := range wantedProcesses {
		processExists(t, true, wantProcess)
	}
	for _, dontWantProcess := range dontWantProcesses {
		processExists(t, false, dontWantProcess)
	}
}

func verifyFileOutput(t *testing.T, event string, corePluginEnabled bool) {
	t.Helper()
	var path string
	var processes string

	wantProcesses, dontWantProcess := getProcessLists(corePluginEnabled)
	if utils.IsWindows() {
		path = fmt.Sprintf(`C:\%s.txt`, event)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read file %q: %v", path, err)
		}
		processes = strings.TrimSpace(string(data))
	} else {
		path = fmt.Sprintf("/home/%s.txt", event)
		processes = readCommands(t, path)
	}

	for _, wantProcess := range wantProcesses {
		if !strings.Contains(processes, wantProcess) {
			t.Errorf("File %q does not contain process %q, found processes:\n %s", path, wantProcess, processes)
		}
	}
	for _, dontWantProcess := range dontWantProcess {
		if strings.Contains(processes, dontWantProcess) {
			t.Errorf("File %q contains unexpected process %q, found processes:\n %s", path, dontWantProcess, processes)
		}
	}
}

func TestDefaultMetadataScriptShutdown(t *testing.T) {
	ctx := context.Background()
	skipIfNoMetadataScriptCompat(t)
	enableAgentDebugLogging(t)

	output := getSerialPortOutput(ctx, t)
	if !strings.Contains(output, "Enable core plugin set to: [false]") {
		t.Errorf("Metadata script compat manager is not enabled. Agent logs: %s", output)
	}

	verifyMetadataScriptsProcesses(t, false)
	verifyFileOutput(t, "shutdown", false)
}

func TestDefaultMetadataScriptStartup(t *testing.T) {
	ctx := context.Background()
	skipIfNoMetadataScriptCompat(t)
	enableAgentDebugLogging(t)

	output := getSerialPortOutput(ctx, t)
	if !strings.Contains(output, "Enable core plugin set to: [false]") {
		t.Errorf("Metadata script compat manager is not enabled. Agent logs: %s", output)
	}

	verifyMetadataScriptsProcesses(t, false)
	verifyFileOutput(t, "startup", false)
}

func TestMetadataScriptCompatStartup(t *testing.T) {
	ctx := context.Background()
	skipIfNoMetadataScriptCompat(t)
	enableAgentDebugLogging(t)

	output := getSerialPortOutput(ctx, t)
	if !strings.Contains(output, "Enable core plugin set to: [true]") {
		t.Errorf("Metadata script compat manager is not enabled. Agent logs: %s", output)
	}

	verifyMetadataScriptsProcesses(t, true)
	verifyFileOutput(t, "startup", true)
}

func TestMetadataScriptCompatShutdown(t *testing.T) {
	ctx := context.Background()
	skipIfNoMetadataScriptCompat(t)
	enableAgentDebugLogging(t)

	output := getSerialPortOutput(ctx, t)
	if !strings.Contains(output, "Enable core plugin set to: [true]") {
		t.Errorf("Metadata script compat manager is not enabled. Agent logs: %s", output)
	}

	verifyMetadataScriptsProcesses(t, true)
	verifyFileOutput(t, "shutdown", true)
}

func readCommands(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Failed to open file %q: %v", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	var foundCommands []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		cmd := strings.Split(line, " ")
		foundCommands = append(foundCommands, cmd[0])
	}

	if scanner.Err() != nil {
		t.Fatalf("Failed to read file %q: %v", path, err)
	}

	return strings.Join(foundCommands, "\n")
}

func getSerialPortOutput(ctx context.Context, t *testing.T) string {
	t.Helper()

	client, err := utils.GetDaisyClient(ctx)
	if err != nil {
		t.Fatalf("Error creating compute service: %v", err)
	}

	instanceName, err := utils.GetInstanceName(ctx)
	if err != nil {
		t.Fatalf("Error getting instance name: %v", err)
	}

	projectID, zone, err := utils.GetProjectZone(ctx)
	if err != nil {
		t.Fatalf("Error getting project or zone: %v", err)
	}

	out, err := client.GetSerialPortOutput(projectID, zone, instanceName, 1, 0)
	if err != nil {
		t.Fatalf("Error getting serial port output: %v", err)
	}

	return out.Contents
}
