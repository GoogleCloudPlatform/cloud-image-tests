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
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func getDriverList(t *testing.T, remove bool) []string {
	drivers := []string{
		"google-compute-engine-driver-gga",
		"google-compute-engine-driver-balloon",
	}

	isWin2012 := isWindowsServer2012Image(t)

	// pvpanic is not typically installed or required on Windows Server 2012.
	if !isWin2012 {
		drivers = append(drivers, "google-compute-engine-driver-pvpanic")
	} else {
		t.Logf("Running on Windows Server 2012, skipping 'google-compute-engine-driver-pvpanic' check.")
	}

	// Do not remove Network or Disk
	if !remove {
		drivers = append(
			drivers,
			"google-compute-engine-driver-gvnic",
			"google-compute-engine-driver-netkvm",
			"google-compute-engine-driver-vioscsi",
		)
	}
	return drivers
}

func isWindowsServer2012Image(t *testing.T) bool {
	imageName, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Logf("Warning: Could not retrieve image name from metadata: %v. Proceeding with all driver checks.", err)
		return false
	}

	return strings.Contains(imageName, "windows-2012")
}

func getInstalledDrivers() ([]string, error) {
	command := fmt.Sprintf("%s installed | Select-String google-compute-engine-driver-", googet)
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		return nil, err
	}

	drivers := strings.Split(output.Stdout, "\n")

	return drivers, nil

}

func TestVirtIONetworkDriverLoaded(t *testing.T) {
	utils.WindowsOnly(t)
	command := fmt.Sprintf("ipconfig /all | Select-String Description")
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error running 'ipconfig': %v", err)
	}
	adapterName := "Google VirtIO Ethernet Adapter"
	if !strings.Contains(output.Stdout, adapterName) {
		t.Fatalf("Stdout: %s does not contain '%s'", output.Stdout, adapterName)
	}
}

func TestDriversInstalled(t *testing.T) {
	utils.WindowsOnly(t)
	driverList := getDriverList(t, false)
	installedDriverList, err := getInstalledDrivers()
	if err != nil {
		t.Fatalf("Cannot get installed drivers list: %v", err)
	}
	for _, driver := range driverList {
		driverInstalled := false
		for _, installed := range installedDriverList {
			if strings.Contains(installed, driver) {
				driverInstalled = true
				break
			}
		}
		if !driverInstalled {
			t.Fatalf("Driver '%s' is not installed", driver)
		}
	}
}

func TestDriversRemoved(t *testing.T) {
	utils.WindowsOnly(t)
	driverList := getDriverList(t, true)
	for _, driver := range driverList {
		command := fmt.Sprintf("%s -noconfirm remove %s", googet, driver)
		output, err := utils.RunPowershellCmd(command)
		if err != nil {
			t.Fatalf("Error removing '%s': %v", driver, err)
		}
		rmString := fmt.Sprintf("Removal of %s completed", driver)
		if !strings.Contains(output.Stdout, rmString) {
			t.Fatalf("Cannot confirm removal of '%s': %s", driver, output.Stdout)
		}
	}
}
