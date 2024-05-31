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
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	googet      = "C:\\ProgramData\\GooGet\\googet.exe"
	repoPath    = "C:\\ProgramData\\GooGet\\repos\\google-compute-engine-testing.repo"
	repoContent = `"- name: google-compute-engine-testing
  url: https://packages.cloud.google.com/yuck/repos/google-compute-engine-testing
  useoauth: true"`
)

func ChangeRepo(t *testing.T) {
	command := fmt.Sprintf("cmd.exe /c del /Q C:\\ProgramData\\GooGet\\repos\\*")
	utils.FailOnPowershellFail(command, "Error deleting stable repo", t)

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

func TestDriverUpgrade(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)
	drivers := []string{
		"google-compute-engine-driver-pvpanic",
		"google-compute-engine-driver-gga",
		"google-compute-engine-driver-balloon",
		"google-compute-engine-driver-gvnic",
		/*
			The driver packages will need to be updated to accomodate this test behavior.
			We want to uncomment the following drivers once they are fixed,
			"google-compute-engine-driver-netkvm",
			"google-compute-engine-driver-vioscsi",
		*/
	}

	for _, driver := range drivers {
		command := fmt.Sprintf("%s installed %s", googet, driver)
		output, err := utils.RunPowershellCmd(command)
		if err != nil {
			t.Fatalf("Error getting package status for '%s'", driver)
		}
		inString := fmt.Sprintf("No package matching filter \"%s\" installed.", driver)
		if !strings.Contains(output.Stdout, inString) {
			command := fmt.Sprintf("%s -noconfirm install -reinstall %s", googet, driver)
			output, err := utils.RunPowershellCmd(command)
			if err != nil {
				t.Fatalf("Error reinstalling '%s': %v", driver, err)
			}
			reString := fmt.Sprintf("Reinstallation of %s.* completed", driver)
			matched, err := regexp.MatchString(reString, output.Stdout)
			if !matched {
				t.Fatalf("Reinstall of '%s' returned unexpected result: %s", driver, output.Stdout)
			}
		} else {
			command := fmt.Sprintf("%s -noconfirm install %s", googet, driver)
			output, err := utils.RunPowershellCmd(command)
			if err != nil {
				t.Fatalf("Error installing '%s': %v", driver, err)
			}
			reString := fmt.Sprintf("Installation of %s.* and all dependencies completed", driver)
			matched, err := regexp.MatchString(reString, output.Stdout)
			if !matched {
				t.Fatalf("Install of '%s' returned unexpected result: %s", driver, output.Stdout)
			}
		}
	}
}

func TestPackageUpgrade(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)
	packages := []string{
		"certgen",
		"googet",
		//Ops Agent uses its own repo; need to make a separate change to test it
		//"google-cloud-ops-agent",
		"google-compute-engine-diagnostics",
		"google-compute-engine-metadata-scripts",
		"google-compute-engine-powershell",
		"google-compute-engine-sysprep",
		"google-compute-engine-windows",
		"google-osconfig-agent",
	}

	for _, agent := range packages {
		command := fmt.Sprintf("%s installed %s", googet, agent)
		output, err := utils.RunPowershellCmd(command)
		if err != nil {
			t.Fatalf("Error getting package status for '%s': %s", agent, err)
		}
		inString := fmt.Sprintf("No package matching filter \"%s\" installed.", agent)
		if !strings.Contains(output.Stdout, inString) {
			command := fmt.Sprintf("%s -noconfirm install -reinstall %s", googet, agent)
			_, err := utils.RunPowershellCmd(command)
			if err != nil {
				t.Fatalf("Error reinstalling '%s': %v", agent, err)
			}
		} else {
			command := fmt.Sprintf("%s -noconfirm install %s", googet, agent)
			_, err := utils.RunPowershellCmd(command)
			if err != nil {
				t.Fatalf("Error installing '%s': %v", agent, err)
			}
		}
	}
}
