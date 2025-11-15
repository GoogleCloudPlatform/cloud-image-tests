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

// Package compatmanager contains tests for compat manager.
package compatmanager

import (
	"fmt"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/exceptions"
	"github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "compatmanager"

const (
	linuxStartupScript = `
#!/bin/bash

ps -eo command >> /home/startup.txt
sync
`
	linuxShutdownScript = `
#!/bin/bash

ps -eo command >> /home/shutdown.txt
sync
`

	windowsStartupScript = `
Get-Process | Format-List * | Out-File -FilePath 'C:\startup.txt' -Encoding ASCII
`
	windowsShutdownScript = `
Get-Process | Format-List * | Out-File -FilePath 'C:\shutdown.txt' -Encoding ASCII
`
	windowsSysprepScript = `
Get-Process | Format-List * | Out-File -FilePath 'C:\sysprep.txt' -Encoding ASCII
`
)

var ubuntuExceptions = []exceptions.Exception{
	{
		Match:   "ubuntu-2404-lts-(amd64|arm64)-guest-agent-stable.*",
		Version: 2404,
	},
	{
		Match:   "ubuntu-2504-(amd64|arm64)-guest-agent-stable.*",
		Version: 2504,
	},
}

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	// TODO(b/460869613): Re-enable compat manager tests on Windows once the bug is fixed.
	if utils.HasFeature(t.Image, "WINDOWS") {
		t.Skip("Compat manager is temporarily disabled on Windows images.")
		return nil
	}

	hasException := exceptions.HasMatch(t.Image.Name, ubuntuExceptions)

	if hasException {
		t.Skip(fmt.Sprintf("Skipping test for image %q, derived image does not have compat manager disabled", t.Image.Name))
		return nil
	}

	defaultVM, err := t.CreateTestVM("compatmanager")
	if err != nil {
		return err
	}
	defaultVM.AddScope("https://www.googleapis.com/auth/cloud-platform")
	defaultVM.RunTests("TestCompatManager")

	// Test metadata script compat manager with startup script.
	metadatStartupTestVM, err := t.CreateTestVM("compatmanagermetadatastartup")
	if err != nil {
		return err
	}
	metadatStartupTestVM.AddScope("https://www.googleapis.com/auth/cloud-platform")
	metadatStartupTestVM.AddMetadata("enable-guest-agent-core-plugin", "true")

	// Default metadata script runner without compat manager.
	defaultStartupTestVM, err := t.CreateTestVM("defaultmetadatastartup")
	if err != nil {
		return err
	}
	defaultStartupTestVM.AddScope("https://www.googleapis.com/auth/cloud-platform")

	// Test metadata script compat manager with shutdown script.
	metadataShutdownTestVM := &daisy.Instance{}
	metadataShutdownTestVM.Metadata = map[string]string{imagetest.ShouldRebootDuringTest: "true"}
	rebootVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "shutdownscripts"}}, metadataShutdownTestVM)
	if err != nil {
		return err
	}
	rebootVM.AddScope("https://www.googleapis.com/auth/cloud-platform")
	rebootVM.AddMetadata("enable-guest-agent-core-plugin", "true")

	// Default shutdown script runner without compat manager.
	defaultShutdownTestVM := &daisy.Instance{}
	defaultShutdownTestVM.Metadata = map[string]string{imagetest.ShouldRebootDuringTest: "true"}
	defaultRebootVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "defaultshutdownscripts"}}, defaultShutdownTestVM)
	if err != nil {
		return err
	}
	defaultRebootVM.AddScope("https://www.googleapis.com/auth/cloud-platform")

	var metadataSysprepTestVM *imagetest.TestVM
	var defaultSysprepTestVM *imagetest.TestVM

	if utils.HasFeature(t.Image, "WINDOWS") {
		// Test metadata script compat manager with sysprep script.
		metadataSysprepTestVM, err = t.CreateTestVM("sysprepspecialize")
		if err != nil {
			return err
		}
		metadataSysprepTestVM.AddMetadata("enable-guest-agent-core-plugin", "true")
		metadataSysprepTestVM.AddScope("https://www.googleapis.com/auth/cloud-platform")

		defaultSysprepTestVM, err = t.CreateTestVM("defaultsysprep")
		if err != nil {
			return err
		}
		defaultSysprepTestVM.AddScope("https://www.googleapis.com/auth/cloud-platform")

		metadatStartupTestVM.SetWindowsStartupScript(windowsStartupScript)
		rebootVM.SetWindowsShutdownScript(windowsShutdownScript)
		metadataSysprepTestVM.AddMetadata("sysprep-specialize-script-ps1", windowsSysprepScript)

		defaultStartupTestVM.SetWindowsStartupScript(windowsStartupScript)
		defaultRebootVM.SetWindowsShutdownScript(windowsShutdownScript)
		defaultSysprepTestVM.AddMetadata("sysprep-specialize-script-ps1", windowsSysprepScript)
	} else {
		rebootVM.SetShutdownScript(linuxShutdownScript)
		metadatStartupTestVM.SetStartupScript(linuxStartupScript)

		defaultStartupTestVM.SetStartupScript(linuxStartupScript)
		defaultRebootVM.SetShutdownScript(linuxShutdownScript)
	}

	if err := rebootVM.Reboot(); err != nil {
		return err
	}
	if err := defaultRebootVM.Reboot(); err != nil {
		return err
	}

	metadatStartupTestVM.RunTests("TestMetadataScriptCompatStartup")
	rebootVM.RunTests("TestMetadataScriptCompatShutdown")

	defaultStartupTestVM.RunTests("TestDefaultMetadataScriptStartup")
	defaultRebootVM.RunTests("TestDefaultMetadataScriptShutdown")

	if utils.HasFeature(t.Image, "WINDOWS") {
		metadataSysprepTestVM.RunTests("TestMetadataScriptCompatSysprep")
		defaultSysprepTestVM.RunTests("TestDefaultMetadataScriptSysprep")
	}

	return nil
}
