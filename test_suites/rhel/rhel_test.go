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

	isEUSOrSAP := strings.Contains(image, "sap") || strings.Contains(image, "eus")
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
			t.Fatalf("Failed to get the rhel-version metadata: %v", err)
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

	isBYOS := strings.Contains(image, "byos")
	isEUS := strings.Contains(image, "eus")
	isSAP := strings.Contains(image, "sap")

	expectedMajorVersion, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "rhel-major-version")
	if err != nil {
		t.Fatalf("Failed to get the rhel-version metadata: %v", err)
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
		if strings.Contains(string(output), "-eus") || strings.Contains(string(output), "-sap") {
			t.Errorf("The rhui client package is not installed: %s", err)
		}
	}
}
