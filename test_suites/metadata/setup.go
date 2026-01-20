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
	metadataMaxLength      = 32768
	shutdownScriptLinuxURL = "scripts/shutdownScriptLinux.sh"
	startupScriptLinuxURL  = "scripts/startupScriptLinux.sh"
	daemonScriptLinuxURL   = "scripts/daemonScriptLinux.sh"
	daemonScriptWindowsURL = "scripts/daemonScriptWindows.ps1"

	// The following scripts are used to test the scripts on Windows.
	ps1Cmd = `Invoke-RestMethod -Method Put -Body %[1]s-success -Headers @{'Metadata-Flavor' = 'Google'} -Uri 'http://metadata.google.internal/computeMetadata/v1/instance/guest-attributes/testing/%[1]s-result' -ContentType 'application/json; charset=utf-8' -UseBasicParsing`
	cmdCmd = `pwsh -Command "Invoke-RestMethod -Method Put -Body %[1]s-success -Headers @{'Metadata-Flavor' = 'Google'} -Uri 'http://metadata.google.internal/computeMetadata/v1/instance/guest-attributes/testing/%[1]s-result' -ContentType 'application/json; charset=utf-8' -UseBasicParsing"`
	batCmd = `curl -X PUT -H "Metadata-Flavor: Google" -H "Content-Type: application/json; charset=utf-8" --data "%[1]s-success" "http://metadata.google.internal/computeMetadata/v1/instance/guest-attributes/testing/%[1]s-result"`
)

//go:embed *
var scripts embed.FS

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {

	vm, err := t.CreateTestVM("mdscommunication")
	if err != nil {
		return err
	}

	vm2Inst := &daisy.Instance{}
	vm2Inst.Metadata = map[string]string{imagetest.ShouldRebootDuringTest: "true"}
	shutdownScriptsVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "shutdownscripts"}}, vm2Inst)
	if err != nil {
		return err
	}
	shutdownScriptsVM.AddMetadata("enable-guest-attributes", "TRUE")
	if err := shutdownScriptsVM.Reboot(); err != nil {
		return err
	}

	vm3Inst := &daisy.Instance{}
	vm3Inst.Metadata = map[string]string{imagetest.ShouldRebootDuringTest: "true"}
	shutdownScriptsMaxLengthVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "shutdownscriptsfailed"}}, vm3Inst)
	if err != nil {
		return err
	}
	shutdownScriptsMaxLengthVM.AddMetadata("enable-guest-attributes", "TRUE")
	if err := shutdownScriptsMaxLengthVM.Reboot(); err != nil {
		return err
	}

	startupScriptsVM, err := t.CreateTestVM("startupscripts")
	if err != nil {
		return err
	}
	startupScriptsVM.AddMetadata("enable-guest-attributes", "TRUE")

	startupScriptsMaxLengthVM, err := t.CreateTestVM("startupscriptsfailed")
	if err != nil {
		return err
	}
	startupScriptsMaxLengthVM.AddMetadata("enable-guest-attributes", "TRUE")

	startupScriptsDaemonVM, err := t.CreateTestVM("daemonscripts")
	if err != nil {
		return err
	}
	startupScriptsDaemonVM.AddMetadata("enable-guest-attributes", "TRUE")

	var startupByteArr []byte
	var shutdownByteArr []byte
	var daemonByteArr []byte

	// Determine if the OS is Windows or Linux and set the appropriate script metadata.
	if utils.HasFeature(t.Image, "WINDOWS") {
		daemonByteArr, err = scripts.ReadFile(daemonScriptWindowsURL)
		if err != nil {
			return err
		}
		daemonScript := string(daemonByteArr)

		// Windows shutdown scripts.
		shutdownScriptsVM.SetWindowsShutdownScript(fmt.Sprintf(ps1Cmd, "shutdown-ps1"))
		shutdownScriptsVM.AddMetadata("windows-shutdown-script-cmd", fmt.Sprintf(cmdCmd, "shutdown-cmd"))
		shutdownScriptsVM.AddMetadata("windows-shutdown-script-bat", fmt.Sprintf(batCmd, "shutdown-bat"))
		shutdownScriptsVM.SetWindowsShutdownScriptURL(fmt.Sprintf(ps1Cmd, "shutdown-url"))
		shutdownScriptsMaxLengthVM.SetWindowsShutdownScript(strings.Repeat("a", metadataMaxLength))

		// Windows startup scripts.
		startupScriptsVM.SetWindowsStartupScript(fmt.Sprintf(ps1Cmd, "startup-ps1"))
		startupScriptsVM.AddMetadata("windows-startup-script-cmd", fmt.Sprintf(cmdCmd, "startup-cmd"))
		startupScriptsVM.AddMetadata("windows-startup-script-bat", fmt.Sprintf(batCmd, "startup-bat"))
		startupScriptsMaxLengthVM.SetWindowsStartupScript(strings.Repeat("a", metadataMaxLength))
		startupScriptsDaemonVM.SetWindowsStartupScript(daemonScript)

		// Windows sysprep scripts.
		sysprepspecialize, err := t.CreateTestVM("sysprepspecialize")
		if err != nil {
			return err
		}
		sysprepspecialize.AddMetadata("enable-guest-attributes", "TRUE")
		sysprepspecialize.AddMetadata("sysprep-specialize-script-ps1", fmt.Sprintf(ps1Cmd, "sysprep-ps1", "sysprep-ps1"))
		sysprepspecialize.AddMetadata("sysprep-specialize-script-cmd", fmt.Sprintf(cmdCmd, "sysprep-cmd"))
		sysprepspecialize.AddMetadata("sysprep-specialize-script-bat", fmt.Sprintf(batCmd, "sysprep-bat"))
		sysprepspecialize.SetWindowsSysprepScriptURL(fmt.Sprintf(ps1Cmd, "sysprep-url", "sysprep-url"))
		sysprepspecialize.RunTests("TestSysprepSpecialize")
	} else {
		startupByteArr, err = scripts.ReadFile(startupScriptLinuxURL)
		if err != nil {
			return err
		}
		shutdownByteArr, err = scripts.ReadFile(shutdownScriptLinuxURL)
		if err != nil {
			return err
		}
		daemonByteArr, err = scripts.ReadFile(daemonScriptLinuxURL)
		if err != nil {
			return err
		}
		startupScript := string(startupByteArr)
		shutdownScript := string(shutdownByteArr)
		daemonScript := string(daemonByteArr)

		// For Linux, create a dedicated VM to test the shutdown script URL.
		shutdownScriptURLInst := &daisy.Instance{}
		shutdownScriptURLInst.Metadata = map[string]string{imagetest.ShouldRebootDuringTest: "true"}
		shutdownScriptURL, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "shutdownurlscripts"}}, shutdownScriptURLInst)
		if err != nil {
			return err
		}
		if err := shutdownScriptURL.Reboot(); err != nil {
			return err
		}
		shutdownScriptURL.AddMetadata("enable-guest-attributes", "TRUE")
		shutdownScriptURL.RunTests("TestShutdownURLScripts")

		shutdownScriptsVM.SetShutdownScript(shutdownScript)
		shutdownScriptsMaxLengthVM.SetShutdownScript(strings.Repeat("a", metadataMaxLength))
		shutdownScriptURL.SetShutdownScriptURL(shutdownScript)
		startupScriptsVM.SetStartupScript(startupScript)
		startupScriptsMaxLengthVM.SetStartupScript(strings.Repeat("a", metadataMaxLength))
		startupScriptsDaemonVM.SetStartupScript(daemonScript)
	}

	tests := "TestTokenFetch|TestGetMetaDataUsingIP"
	if !t.IsComputeStaging() {
		// Skip TestMetaDataResponseHeaders in staging until MDS issue is resolved.
		// These additional headers are not seen in prod.
		// TODO(b/431975346): Re-enable test once MDS issue is resolved.
		tests = tests + "|TestMetaDataResponseHeaders"
	}
	// Run the tests after setup is complete.
	vm.RunTests(tests)
	shutdownScriptsVM.RunTests("TestShutdownScripts")
	shutdownScriptsMaxLengthVM.RunTests("TestShutdownScriptsFailed")
	startupScriptsVM.RunTests("TestStartupScripts")
	startupScriptsMaxLengthVM.RunTests("TestStartupScriptsFailed")
	startupScriptsDaemonVM.RunTests("TestDaemonScripts")

	startupCustomURLPatterns := &daisy.Instance{}
	startupCustomURLPatternsVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "startupCustomURLPatterns"}}, startupCustomURLPatterns)
	if err != nil {
		return err
	}

	startupCustomURLPatternsVM.AddScope("https://www.googleapis.com/auth/compute") // Compute scope is needed for setting metadata.
	startupCustomURLPatternsVM.RunTests("TestCustomURLPatterns")

	return nil
}
