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

package rhel

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	releaseVerFile = "/etc/dnf/vars/releasever"
)

func TestVersionLock(t *testing.T) {
	utils.LinuxOnly(t)
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Failed to read image metadata: %v", err)
	}

	isEUSOrSAP := utils.IsSAP(image) || utils.IsRHELEUS(image)
	data, err := os.ReadFile(releaseVerFile)
	if err != nil {
		t.Fatalf("Failed to read releasever file: %v", err)
	}

	if isEUSOrSAP {
		rhelVersionSplit := strings.Split(string(data), ".")
		rhelMajorVersion := strings.TrimSpace(rhelVersionSplit[0])
		rhelMinorVersion := strings.TrimSpace(rhelVersionSplit[1])

		expectedMajorVersion, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "rhel-major-version")
		if err != nil {
			t.Fatalf("Failed to get the rhel-major-version metadata: %v", err)
		}

		expectedMinorVersion, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "rhel-minor-version")
		if err != nil {
			t.Fatalf("Failed to get the rhel-minor-version metadata: %v", err)
		}

		if rhelMajorVersion != expectedMajorVersion {
			t.Errorf("The major release version in the image does not match the major release version in the image family: %s != %s", rhelMajorVersion, expectedMajorVersion)
		}

		if rhelMinorVersion != expectedMinorVersion {
			t.Errorf("The minor release version in the image does not match the minor release version in the image family: \"%s\" != \"%s\"", rhelMinorVersion, expectedMinorVersion)
		}
	} else {
		if err == nil {
			t.Errorf("The release version file shouldn't exist for non-EUS/SAP images: %s", string(data))
		}
	}
}

func TestRhuiPackage(t *testing.T) {
	utils.LinuxOnly(t)
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Failed to read image metadata: %v", err)
	}

	isBYOS := utils.IsBYOS(image)
	isEUS := utils.IsRHELEUS(image)
	isSAP := utils.IsSAP(image)

	expectedMajorVersion, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "rhel-major-version")
	if err != nil {
		t.Fatalf("Failed to get the rhel-major-version metadata: %v", err)
	}

	rhuiClientPackage := "google-rhui-client-rhel" + expectedMajorVersion
	// For the final point release of a major version for EUS/SAP images, the package appends the
	// minor release version.
	rhuiClientFinalPackage := rhuiClientPackage + "10"
	if isEUS {
		rhuiClientPackage = rhuiClientPackage + "-eus"
		rhuiClientFinalPackage = rhuiClientPackage + "-eus"
	} else if isSAP {
		rhuiClientPackage = rhuiClientPackage + "-sap"
		rhuiClientFinalPackage = rhuiClientPackage + "-sap"
	}
	output, err := exec.Command("rpm", "-q", rhuiClientPackage).Output()

	if isBYOS {
		if err == nil {
			t.Errorf("The rhui client package shouldn't still be installed")
		}
	} else if isEUS || isSAP {
		if !strings.Contains(string(output), rhuiClientPackage) && !strings.Contains(string(output), rhuiClientFinalPackage) {
			t.Errorf("The rhui client package is not installed: %s", err)
		}
	} else {
		if !strings.Contains(string(output), rhuiClientPackage) {
			t.Errorf("The rhui client package is not installed: %s", err)
		}
		// Since "-eus" and "-sap" are appended to the base rhui package name, if we are checking for
		// the base RHEL image then make sure that the package name doesn't contain "-eus" or "-sap".
		if utils.IsRHELEUS(string(output)) || utils.IsSAP(string(output)) {
			t.Errorf("The rhui client package is not installed: %s", err)
		}
	}
}

func TestPackageInstallation(t *testing.T) {
	utils.LinuxOnly(t)
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Failed to read image metadata: %v", err)
	}

	isArm := strings.Contains(image, "arm")
	isBYOS := utils.IsBYOS(image)
	isSAP := utils.IsSAP(image)

	expectedMajorVersion, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "rhel-major-version")
	if err != nil {
		t.Fatalf("Failed to get the rhel-major-version metadata: %v", err)
	}
	expectedMinorVersion, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "rhel-minor-version")
	if isSAP && err != nil {
		t.Fatalf("Failed to get the rhel-minor-version metadata: %v", err)
	}

	// nvme-cli is needed on all RHEL images.
	checkPackageInstallation(t, "nvme-cli", true)
	// subscription-manager is installed on BYOS images only.
	checkPackageInstallation(t, "subscription-manager", isBYOS)

	switch expectedMajorVersion {
	case "10":
		// RHEL 10 w/ rhc: All BYOS
		// RHEL 10 w/ insights-client: All BYOS
		checkPackageInstallation(t, "rhc", isBYOS)
		checkPackageInstallation(t, "insights-client", isBYOS)
	case "9":
		isSAP90 := isSAP && expectedMinorVersion == "0"
		isSAP92 := isSAP && expectedMinorVersion == "2"
		// RHEL 9 w/ rhc: All BYOS (except RHEL 9.0 SAP BYOS), RHEL 9.2 SAP.
		if (isBYOS && !isSAP90) || isSAP92 {
			checkPackageInstallation(t, "rhc", true)
		} else {
			checkPackageInstallation(t, "rhc", false)
		}
		// RHEL 9 w/ insights-client: All BYOS (except RHEL 9.0 SAP BYOS), RHEL 9.2 SAP.
		if (isBYOS && !isSAP90) || isSAP92 {
			checkPackageInstallation(t, "insights-client", true)
		} else {
			checkPackageInstallation(t, "insights-client", false)
		}
	case "8":
		isSAP86 := isSAP && expectedMinorVersion == "6"
		isSAP88 := isSAP && expectedMinorVersion == "8"
		isBYOSX86 := isBYOS && !isArm

		// RHEL 8 w/ rhc: All x86 BYOS, RHEL 8.6/8.8 SAP.
		if isSAP86 || isSAP88 || isBYOSX86 {
			checkPackageInstallation(t, "rhc", true)
		} else {
			checkPackageInstallation(t, "rhc", false)
		}

		// RHEL 8 w/ insights-client: All x86 BYOS, RHEL 8.6/8.8 SAP.
		if isSAP86 || isSAP88 || isBYOSX86 {
			checkPackageInstallation(t, "insights-client", true)
		} else {
			checkPackageInstallation(t, "insights-client", false)
		}
	default:
		t.Fatalf("Unsupported RHEL major version: %s", expectedMajorVersion)
	}
}

func checkPackageInstallation(t *testing.T, packageName string, shouldBeInstalled bool) {
	t.Helper()
	output, err := exec.Command("rpm", "-q", packageName).Output()
	if shouldBeInstalled {
		if err != nil {
			t.Errorf("The package %s is not installed when it should be: %s", packageName, string(output))
		}
	} else {
		if err == nil {
			t.Errorf("The package %s is still installed when it should not be: %s", packageName, string(output))
		}
	}
}
