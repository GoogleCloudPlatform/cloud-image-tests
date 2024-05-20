package packageupgrade

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	googet      = "C:\\ProgramData\\GooGet\\googet.exe"
	stagingRepo = "https://packages.cloud.google.com/yuck/repos/google-compute-engine-staging"
)

func ChangeRepo(t *testing.T) {
	utils.WindowsOnly(t)
	command := fmt.Sprintf("cmd.exe /c del /Q C:\\ProgramData\\GooGet\\repos\\*")
	utils.FailOnPowershellFail(command, "Error deleting stable repo", t)

	command = fmt.Sprintf("%s available", googet)
	err := utils.CheckPowershellReturnCode(command, 1)
	if err != nil {
		t.Fatal("%s: %v", command, err)
	}

	command = fmt.Sprintf("%s addrepo gce-stable %s", googet, stagingRepo)
	utils.FailOnPowershellFail(command, "Error adding staging repo", t)
}

func TestDriverUpgrade(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)
	drivers := []string{
		"google-compute-engine-driver-pvpanic",
		"google-compute-engine-driver-gga",
		"google-compute-engine-driver-balloon",
		"google-compute-engine-driver-gvnic",
		"google-compute-engine-driver-netkvm",
		"google-compute-engine-driver-vioscsi",
	}

	for _, driver := range drivers {
		command := fmt.Sprintf("%s -noconfirm install -reinstall %s", googet, driver)
		output, err := utils.RunPowershellCmd(command)
		if err != nil {
			t.Fatalf("Error reinstalling '%s': %v", driver, err)
		}
		reString := fmt.Sprintf("Reinstallation of %s completed", driver)
		if !strings.Contains(output.Stdout, reString) {
			t.Fatalf("Reinstall of '%s' returned unexpected result: %s", driver, output.Stdout)
		}
	}
}

func TestPackageUpgrade(t *testing.T) {
	utils.WindowsOnly(t)
	ChangeRepo(t)
	packages := []string{
		"certgen",
		"googet",
		"google-cloud-ops-agent",
		"google-compute-engine-diagnostics",
		"google-compute-engine-metadata-scripts",
		"google-compute-engine-powershell",
		"google-compute-engine-sysprep",
		"google-compute-engine-windows",
		"google-osconfig-agent",
	}

	for _, package := range packages {
		command := fmt.Sprintf("%s -noconfirm install -reinstall %s", googet, package)
		output, err := utils.RunPowershellCmd(command)
		if err != nil {
			t.Fatalf("Error reinstalling '%s': %v", package, err)
		}
		reString := fmt.Sprintf("Reinstallation of %s completed", package)
		if !strings.Contains(output.Stdout, reString) {
			t.Fatalf("Reinstall of '%s' returned unexpected result: %s", package, output.Stdout)
		}
	}
}
