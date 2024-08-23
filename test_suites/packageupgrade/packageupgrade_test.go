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

package packageupgrade

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	googet      = "C:\\ProgramData\\GooGet\\googet.exe"
	repoPath    = "C:\\ProgramData\\GooGet\\repos\\google-compute-engine-testing.repo"
	repoContent = `"- name: google-compute-engine-testing
  url: https://packages.cloud.google.com/yuck/repos/google-compute-engine-testing
  useoauth: true"`
	cacheErr = "cache either doesn't exist or is older than 3m0s"
)

func ChangeRepo(t *testing.T) {
	command := fmt.Sprintf("cmd.exe /c del /Q C:\\ProgramData\\GooGet\\repos\\*")
	utils.FailOnPowershellFail(command, "Error deleting stable repo.", t)

	command = fmt.Sprintf("%s available", googet)
	err := utils.CheckPowershellReturnCode(command, 1)
	if err != nil {
		t.Fatal(err)
	}

	command = fmt.Sprintf("Set-Content -Path %s -Value %s", repoPath, repoContent)
	_, err = utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatal(err)
	}
}

func VerifyInstallReturn(t *testing.T, pkg string, input string) {
	installString := fmt.Sprintf("Installation of %s.* and all dependencies completed", pkg)
	noactionString := fmt.Sprintf("%s.* or a newer version is already installed on the system", pkg)
	matchedInstall, err := regexp.MatchString(installString, input)
	if err != nil {
		t.Fatalf("Failed to parse package install result: %v", err)
	}
	matchedNoaction, err := regexp.MatchString(noactionString, input)
	if err != nil {
		t.Fatalf("Failed to parse package install result: %v", err)
	}
	if matchedInstall || matchedNoaction {
		return
	}
	t.Fatalf("Installation of '%s' returned unexpected result: %s", pkg, input)
}

func TestPvpanicDriverInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	driver := "google-compute-engine-driver-pvpanic"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, driver)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", driver, output.Stdout)
	}
	VerifyInstallReturn(t, driver, output.Stdout)
}

func TestGgaDriverInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	driver := "google-compute-engine-driver-gga"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, driver)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", driver, output.Stdout)
	}
	VerifyInstallReturn(t, driver, output.Stdout)
}

func TestBalloonDriverInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	driver := "google-compute-engine-driver-balloon"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, driver)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", driver, output.Stdout)
	}
	VerifyInstallReturn(t, driver, output.Stdout)
}

func TestGvnicDriverInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	driver := "google-compute-engine-driver-gvnic"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, driver)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", driver, output.Stdout)
	}
	VerifyInstallReturn(t, driver, output.Stdout)
}

func TestNetkvmDriverInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	driver := "google-compute-engine-driver-netkvm"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, driver)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", driver, output.Stdout)
	}
	VerifyInstallReturn(t, driver, output.Stdout)
}

func TestVioscsiDriverInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	driver := "google-compute-engine-driver-vioscsi"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, driver)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", driver, output.Stdout)
	}
	VerifyInstallReturn(t, driver, output.Stdout)
}

func TestCertgenPackageInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	pkg := "certgen"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, pkg)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", pkg, output.Stdout)
	}
	VerifyInstallReturn(t, pkg, output.Stdout)
}

func TestGoogetPackageInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	pkg := "googet"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, pkg)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", pkg, output.Stdout)
	}
	VerifyInstallReturn(t, pkg, output.Stdout)
}

func TestGceDiagnosticsPackageInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	pkg := "google-compute-engine-diagnostics"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, pkg)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", pkg, output.Stdout)
	}
	VerifyInstallReturn(t, pkg, output.Stdout)
}

func TestGceMetadataScriptsPackageInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	pkg := "google-compute-engine-metadata-scripts"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, pkg)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", pkg, output.Stdout)
	}
	VerifyInstallReturn(t, pkg, output.Stdout)
}

func TestGcePowershellPackageInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	pkg := "google-compute-engine-powershell"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, pkg)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", pkg, output.Stdout)
	}
	VerifyInstallReturn(t, pkg, output.Stdout)
}

func TestGceSysprepPackageInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	pkg := "google-compute-engine-sysprep"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, pkg)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", pkg, output.Stdout)
	}
	VerifyInstallReturn(t, pkg, output.Stdout)
}

func TestWindowsGuestAgentInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	pkg := "google-compute-engine-windows"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, pkg)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", pkg, output.Stdout)
	}
	VerifyInstallReturn(t, pkg, output.Stdout)
}

func TestOSConfigAgentInstallFromTesting(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	pkg := "google-osconfig-agent"
	command := fmt.Sprintf("%s -noconfirm install %s", googet, pkg)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error installing '%s': %v", pkg, output.Stdout)
	}
	VerifyInstallReturn(t, pkg, output.Stdout)
}
