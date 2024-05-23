package packageupgrade

import (
	"fmt"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	googet      = "C:\\ProgramData\\GooGet\\googet.exe"
	stagingRepo = "https://packages.cloud.google.com/yuck/repos/google-compute-engine-staging"
)

func ChangeRepo(t *testing.T) {
	command := fmt.Sprintf("cmd.exe /c del /Q C:\\ProgramData\\GooGet\\repos\\*")
	utils.FailOnPowershellFail(command, "Error deleting stable repo", t)

	command = fmt.Sprintf("%s available", googet)
	err := utils.CheckPowershellReturnCode(command, 1)
	if err != nil {
		t.Fatal(err)
	}

	command = fmt.Sprintf("%s addrepo gce-stable %s", googet, stagingRepo)
	utils.FailOnPowershellFail(command, "Error adding staging repo", t)
}

/*func TestDriverUpgrade(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)
	drivers := []string{
		"google-compute-engine-driver-pvpanic",
		"google-compute-engine-driver-gga",
		"google-compute-engine-driver-balloon",
		"google-compute-engine-driver-gvnic",
		//"google-compute-engine-driver-netkvm",
		//"google-compute-engine-driver-vioscsi",
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
}*/

func TestPackageUpgrade(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)

	command := fmt.Sprint("googet -noconfirm update")
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error updating googet packages. Stderr: %s", output.Stderr)
	}

	/*packages := []string{
		"certgen",
		"googet",
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
			t.Fatalf("Error getting package status for '%s'", agent)
		}
		inString := fmt.Sprintf("No package matching filter \"%s\" installed.", agent)
		if !strings.Contains(output.Stdout, inString) {
			command := fmt.Sprintf("%s -noconfirm install -reinstall %s", googet, agent)
			output, err := utils.RunPowershellCmd(command)
			if err != nil {
				t.Fatalf("Error reinstalling '%s': %v", agent, err)
			}
			reString := fmt.Sprintf("Reinstallation of %s.* completed", agent)
			matched, err := regexp.MatchString(reString, output.Stdout)
			if !matched {
				t.Fatalf("Reinstall of '%s' returned unexpected result: %s", agent, output.Stdout)
			}
		} else {
			command := fmt.Sprintf("%s -noconfirm install %s", googet, agent)
			output, err := utils.RunPowershellCmd(command)
			if err != nil {
				t.Fatalf("Error installing '%s': %v", agent, err)
			}
			reString := fmt.Sprintf("Installation of %s.* and all dependencies completed", agent)
			matched, err := regexp.MatchString(reString, output.Stdout)
			if !matched {
				t.Fatalf("Install of '%s' returned unexpected result: %s", agent, output.Stdout)
			}
		}
	}*/
}
