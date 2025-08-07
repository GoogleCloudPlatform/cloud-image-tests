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

package packagevalidation

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const googet = "C:\\ProgramData\\GooGet\\googet.exe"
const stableRepo = "https://packages.cloud.google.com/yuck/repos/google-compute-engine-stable"

func getPackageList(image string) []string {
	installedPackages := []string{
		"google-compute-engine-metadata-scripts",
		"google-compute-engine-powershell",
		"google-compute-engine-sysprep",
		"google-compute-engine-windows",
		"certgen",
	}

	if !utils.Is32BitWindows(image) {
		installedPackages = append(
			installedPackages,
			"google-osconfig-agent",
			"google-compute-engine-driver-gvnic",
		)
	}

	if !utils.IsWindowsClient(image) {
		installedPackages = append(
			installedPackages,
			"google-compute-engine-vss",
		)
	}
	return installedPackages
}

func getExecutableList(image string) []string {
	cePath := "C:\\Program Files\\Google\\Compute Engine"
	execList := []string{
		filepath.Join(cePath, "agent\\GCEWindowsAgent.exe"),
		filepath.Join(cePath, "agent\\GCEWindowsAgentManager.exe"),
		filepath.Join(cePath, "agent\\CorePlugin.exe"),
		filepath.Join(cePath, "agent\\GCEMetadataScriptRunner.exe"),
		filepath.Join(cePath, "agent\\GCEWindowsCompatManager.exe"),
		filepath.Join(cePath, "agent\\GCECompatMetadataScripts.exe"),
		filepath.Join(cePath, "agent\\GCEAuthorizedKeysCommand.exe"),
		filepath.Join(cePath, "agent\\GCEAuthorizedKeys.exe"),
		filepath.Join(cePath, "agent\\GCEAuthorizedKeysNew.exe"),
		filepath.Join(cePath, "agent\\ggactl_plugin.exe"),
		filepath.Join(cePath, "metadata_scripts\\GCEMetadataScripts.exe"),
		filepath.Join(cePath, "sysprep\\activate_instance.ps1"),
		filepath.Join(cePath, "sysprep\\sysprep.ps1"),
		filepath.Join(cePath, "sysprep\\instance_setup.ps1"),
		filepath.Join(cePath, "sysprep\\gce_base.psm1"),
		filepath.Join(cePath, "tools\\certgen.exe"),
		"C:\\Program Files\\Google\\OSConfig\\google_osconfig_agent.exe",
	}

	if !utils.IsWindowsClient(image) {
		execList = append(
			execList,
			filepath.Join(cePath, "vss\\GoogleVssAgent.exe"),
			filepath.Join(cePath, "vss\\GoogleVssProvider.dll"),
		)
	}
	return execList
}

func TestGooGetInstalled(t *testing.T) {
	utils.WindowsOnly(t)
	command := fmt.Sprintf("%s installed googet", googet)
	utils.FailOnPowershellFail(command, "GooGet not installed", t)
}

func TestGooGetAvailable(t *testing.T) {
	utils.WindowsOnly(t)
	command := fmt.Sprintf("%s available googet", googet)
	utils.FailOnPowershellFail(command, "GooGet not available", t)
}

func TestSigned(t *testing.T) {
	utils.WindowsOnly(t)
	utils.Skip32BitWindows(t, "Packages not signed on 32-bit client images.")
	command := fmt.Sprintf("(Get-AuthenticodeSignature %s).Status", googet)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Get-AuthenticodeSignature returned an error: %v", err)
	}

	if strings.TrimSpace(output.Stdout) != "Valid" {
		t.Fatalf("GooGet package signature is not valid.")
	}
}

func TestRemoveInstall(t *testing.T) {
	utils.WindowsOnly(t)
	pkg := "google-compute-engine-auto-updater"
	command := fmt.Sprintf("%s -noconfirm remove %s", googet, pkg)
	utils.FailOnPowershellFail(command, "Error removing package", t)

	command = fmt.Sprintf("%s installed %s", googet, pkg)
	err := utils.CheckPowershellReturnCode(command, 1)
	if err != nil {
		t.Fatal(err)
	}

	command = fmt.Sprintf("%s -noconfirm install %s", googet, pkg)
	utils.FailOnPowershellFail(command, "Error installing package", t)

	command = fmt.Sprintf("%s installed %s", googet, pkg)
	utils.FailOnPowershellFail(command, "Package not installed", t)
}

func TestRepoManagement(t *testing.T) {
	utils.WindowsOnly(t)
	command := fmt.Sprintf("cmd.exe /c del /Q C:\\ProgramData\\GooGet\\repos\\*")
	utils.FailOnPowershellFail(command, "Error deleting repos", t)

	command = fmt.Sprintf("%s available googet", googet)
	err := utils.CheckPowershellReturnCode(command, 1)
	if err != nil {
		t.Fatal(err)
	}

	command = fmt.Sprintf("%s addrepo gce-stable %s", googet, stableRepo)
	utils.FailOnPowershellFail(command, "Error adding repo", t)

	command = fmt.Sprintf("%s available googet", googet)
	utils.FailOnPowershellFail(command, "GooGet not available", t)

	command = fmt.Sprintf("%s rmrepo gce-stable", googet)
	utils.FailOnPowershellFail(command, "Error removing repo", t)

	command = fmt.Sprintf("%s available googet", googet)
	err = utils.CheckPowershellReturnCode(command, 1)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPackagesInstalled(t *testing.T) {
	utils.WindowsOnly(t)
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata: %v", err)
	}
	installedPackages := getPackageList(image)
	for _, pkg := range installedPackages {
		command := fmt.Sprintf("%s installed %s", googet, pkg)
		errMsg := fmt.Sprintf("Package %s not installed", pkg)
		utils.FailOnPowershellFail(command, errMsg, t)
	}
}

func TestPackagesAvailable(t *testing.T) {
	utils.WindowsOnly(t)
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata %v", err)
	}
	installedPackages := getPackageList(image)
	for _, pkg := range installedPackages {
		command := fmt.Sprintf("%s available %s", googet, pkg)
		errMsg := fmt.Sprintf("Package %s not available", pkg)
		utils.FailOnPowershellFail(command, errMsg, t)
	}
}

func TestPackagesSigned(t *testing.T) {
	utils.WindowsOnly(t)
	utils.Skip32BitWindows(t, "Packages not signed on 32-bit client images")
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata %v", err)
	}
	// Derived images built by Concourse do not have signed packages.
	if strings.Contains(image, "-guest-agent-") {
		t.Skip("Skip for derived images as packages are not signed.")
	}
	execList := getExecutableList(image)
	for _, exec := range execList {
		if !utils.Exists(exec, utils.TypeFile) {
			// Some executables may not be present based on guest-agent package
			// version.
			t.Logf("Skipping %q as it does not exist", exec)
			continue
		}
		command := fmt.Sprintf("(Get-AuthenticodeSignature '%s').Status", exec)
		output, err := utils.RunPowershellCmd(command)
		if err != nil {
			t.Fatalf("Get-AuthenticodeSignature returned an error: %v", err)
		}

		if strings.TrimSpace(output.Stdout) != "Valid" {
			t.Fatalf("Signature is not valid for %s", exec)
		}
	}
}
