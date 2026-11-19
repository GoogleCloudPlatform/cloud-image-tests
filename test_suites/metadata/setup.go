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

// Package metadata is a CIT suite for testing metadata script functionality.
package metadata

import (
	"embed"
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "metadata"

const (
	// max metadata value 256kb https://cloud.google.com/compute/docs/metadata/setting-custom-metadata#limitations
	// metadataMaxLength = 256 * 1024
	// TODO(hopkiw): above is commented out until error handler is added to
	// output scanner in the script runner. Use smaller size for now.
	metadataMaxLength         = 32768
	shutdownScriptLinuxURL    = "scripts/shutdownScriptLinux.sh"
	shutdownURLScriptLinuxURL = "scripts/shutdownURLScriptLinux.sh"
	startupScriptLinuxURL     = "scripts/startupScriptLinux.sh"
	daemonScriptLinuxURL      = "scripts/daemonScriptLinux.sh"
	daemonScriptWindowsURL    = "scripts/daemonScriptWindows.ps1"

	// The following scripts are used to test the scripts on Windows.
	ps1Cmd = `Invoke-RestMethod -Method Put -Body %[1]s-success -Headers @{'Metadata-Flavor' = 'Google'} -Uri 'http://metadata.google.internal/computeMetadata/v1/instance/guest-attributes/testing/%[1]s-result' -ContentType 'application/json; charset=utf-8' -UseBasicParsing`
	cmdCmd = `pwsh -Command "Invoke-RestMethod -Method Put -Body %[1]s-success -Headers @{'Metadata-Flavor' = 'Google'} -Uri 'http://metadata.google.internal/computeMetadata/v1/instance/guest-attributes/testing/%[1]s-result' -ContentType 'application/json; charset=utf-8' -UseBasicParsing"`
	batCmd = `curl -X PUT -H "Metadata-Flavor: Google" -H "Content-Type: application/json; charset=utf-8" --data "%[1]s-success" "http://metadata.google.internal/computeMetadata/v1/instance/guest-attributes/testing/%[1]s-result"`
)

//go:embed *
var scripts embed.FS

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	isWindows := utils.HasFeature(t.Image, "WINDOWS")

	if err := setupBootVM(t, isWindows); err != nil {
		return err
	}
	if err := setupShutdownVM(t, isWindows); err != nil {
		return err
	}
	return setupShutdownFailedVM(t, isWindows)
}

func readScript(scriptPath string) (string, error) {
	b, err := scripts.ReadFile(scriptPath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func bootTestList(isWindows, isComputeStaging bool) string {
	tests := "TestTokenFetch|TestGetMetaDataUsingIP"
	if !isComputeStaging {
		// Skip TestMetaDataResponseHeaders in staging until MDS issue is resolved.
		// These additional headers are not seen in prod.
		// TODO(b/431975346): Re-enable test once MDS issue is resolved.
		tests += "|TestMetaDataResponseHeaders"
	}
	tests += "|TestStartupScripts|TestDaemonScripts"
	if isWindows {
		tests += "|TestSysprepSpecialize"
	}
	return tests
}

func shutdownTestList(isWindows bool) string {
	if isWindows {
		return "TestShutdownScripts|TestStartupScriptsFailed"
	}
	return "TestShutdownScripts|TestShutdownURLScripts|TestStartupScriptsFailed"
}

// anchorTestRegex wraps a "|"-separated list of exact test names so Go's
// -test.run regexp does not also match tests with shared prefixes (such as
// TestStartupScripts matching TestStartupScriptsFailed).
func anchorTestRegex(tests string) string {
	return fmt.Sprintf("^(%s)$", tests)
}

func createRebootTestVM(t *imagetest.TestWorkflow, name string) (*imagetest.TestVM, error) {
	inst := &daisy.Instance{
		Metadata: map[string]string{imagetest.ShouldRebootDuringTest: "true"},
	}
	vm, err := t.CreateTestVMFromInstance(inst, []*compute.Disk{{Name: name}})
	if err != nil {
		return nil, err
	}
	vm.AddMetadata("enable-guest-attributes", "TRUE")
	if err := vm.Reboot(); err != nil {
		return nil, err
	}
	return vm, nil
}

func setupBootVM(t *imagetest.TestWorkflow, isWindows bool) error {
	tests := bootTestList(isWindows, t.IsComputeStaging())
	if !t.ShouldRunTests(tests) {
		return nil
	}

	vm, err := t.CreateTestVM("mdscommunication")
	if err != nil {
		return err
	}
	vm.AddMetadata("enable-guest-attributes", "TRUE")

	if isWindows {
		daemonScript, err := readScript(daemonScriptWindowsURL)
		if err != nil {
			return err
		}
		vm.SetWindowsStartupScript(fmt.Sprintf(ps1Cmd, "startup-ps1") + "\n" + daemonScript)
		vm.AddMetadata("windows-startup-script-cmd", fmt.Sprintf(cmdCmd, "startup-cmd"))
		vm.AddMetadata("windows-startup-script-bat", fmt.Sprintf(batCmd, "startup-bat"))

		sysprepPS1 := fmt.Sprintf(ps1Cmd, "sysprep-ps1")
		if t.IsComputeStaging() {
			// Preserve skipWindowsStagingKMS when sysprep-specialize-script-ps1 is customized.
			sysprepPS1 = `New-Item -Path "C:\Program Files\Google\Compute Engine\sysprep\byol_image" -ItemType directory` + "\n" + sysprepPS1
		}
		vm.AddMetadata("sysprep-specialize-script-ps1", sysprepPS1)
		vm.AddMetadata("sysprep-specialize-script-cmd", fmt.Sprintf(cmdCmd, "sysprep-cmd"))
		vm.AddMetadata("sysprep-specialize-script-bat", fmt.Sprintf(batCmd, "sysprep-bat"))
		if err := vm.SetWindowsSysprepScriptURL(fmt.Sprintf(ps1Cmd, "sysprep-url")); err != nil {
			return err
		}
	} else {
		startupScript, err := readScript(startupScriptLinuxURL)
		if err != nil {
			return err
		}
		daemonScript, err := readScript(daemonScriptLinuxURL)
		if err != nil {
			return err
		}
		vm.SetStartupScript(startupScript + "\n" + daemonScript)
	}

	vm.RunTests(anchorTestRegex(tests))
	return nil
}

func setupShutdownVM(t *imagetest.TestWorkflow, isWindows bool) error {
	tests := shutdownTestList(isWindows)
	if !t.ShouldRunTests(tests) {
		return nil
	}

	vm, err := createRebootTestVM(t, "shutdownscripts")
	if err != nil {
		return err
	}

	maxLenScript := strings.Repeat("a", metadataMaxLength)
	if isWindows {
		vm.SetWindowsShutdownScript(fmt.Sprintf(ps1Cmd, "shutdown-ps1"))
		vm.AddMetadata("windows-shutdown-script-cmd", fmt.Sprintf(cmdCmd, "shutdown-cmd"))
		vm.AddMetadata("windows-shutdown-script-bat", fmt.Sprintf(batCmd, "shutdown-bat"))
		if err := vm.SetWindowsShutdownScriptURL(fmt.Sprintf(ps1Cmd, "shutdown-url")); err != nil {
			return err
		}
		vm.SetWindowsStartupScript(maxLenScript)
	} else {
		shutdownScript, err := readScript(shutdownScriptLinuxURL)
		if err != nil {
			return err
		}
		shutdownURLScript, err := readScript(shutdownURLScriptLinuxURL)
		if err != nil {
			return err
		}
		vm.SetShutdownScript(shutdownScript)
		if err := vm.SetShutdownScriptURL(shutdownURLScript); err != nil {
			return err
		}
		vm.SetStartupScript(maxLenScript)
	}

	vm.RunTests(anchorTestRegex(tests))
	return nil
}

func setupShutdownFailedVM(t *imagetest.TestWorkflow, isWindows bool) error {
	const tests = "TestShutdownScriptsFailed"
	if !t.ShouldRunTests(tests) {
		return nil
	}

	vm, err := createRebootTestVM(t, "shutdownscriptsfailed")
	if err != nil {
		return err
	}

	maxLenScript := strings.Repeat("a", metadataMaxLength)
	if isWindows {
		vm.SetWindowsShutdownScript(maxLenScript)
	} else {
		vm.SetShutdownScript(maxLenScript)
	}

	vm.RunTests(anchorTestRegex(tests))
	return nil
}
