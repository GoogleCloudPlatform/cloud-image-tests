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

package imageboot

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// The values have been decided based on running spot tests for different images.
var imageFamilyBootTimeThresholdMap = map[string]int{
	"almalinux":   60,
	"centos":      60,
	"debian":      50,
	"rhel":        60,
	"rocky-linux": 60,
	"sles-12":     85,
	"sles-15":     120,
	"ubuntu-pro":  110,
	"ubuntu":      75,
}

const (
	markerFile     = "/boot-marker"
	secureBootFile = "/sys/firmware/efi/efivars/SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c"
	setupModeFile  = "/sys/firmware/efi/efivars/SetupMode-8be4df61-93ca-11d2-aa0d-00e098032b8c"
)

func lookForSshdAndGuestAgentProcess() error {
	dir, _ := os.Open("/proc")
	defer dir.Close()

	names, err := dir.Readdirnames(0)
	if err != nil {
		return err
	}

	var foundSshd bool
	var foundGuestAgent bool

	for _, name := range names {
		// Continue if the directory name does start with a number
		if name[0] < '0' || name[0] > '9' {
			continue
		}

		// Continue if the directory name is not an integer
		_, err := strconv.ParseInt(name, 10, 0)
		if err != nil {
			continue
		}

		// Gets the symbolic link to the executable
		link, err := os.Readlink("/proc/" + name + "/exe")
		if err != nil {
			continue
		}

		if strings.Trim(link, "\n") == "/usr/sbin/sshd" {
			foundSshd = true
		}

		if strings.Trim(link, "\n") == "/usr/bin/google_guest_agent" {
			foundGuestAgent = true
		}

	}

	if foundSshd && foundGuestAgent {
		return nil
	}

	return fmt.Errorf("guest agent and/or sshd not found")
}

func lookForGuestAgentProcessWindows() error {
	command := `$agentservice = Get-Service GCEAgent
	$agentservice.Status`
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		return fmt.Errorf("failed to find Guest Agent service")
	}

	agentStatus := strings.TrimSpace(output.Stdout)
	if agentStatus == "Running" {
		return nil
	}

	return fmt.Errorf("guest agent not found")
}

func verifyBootTime(t *testing.T) error {
	// Reading the system uptime once both guest agent and sshd are found in the processes
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return fmt.Errorf("Failed to read uptime file")
	}
	fields := strings.Split(string(uptimeData), " ")
	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return fmt.Errorf("Failed to read uptime numeric value")
	}
	fmt.Println("found guest agent and sshd running at ", int(uptime), " seconds")

	//Validating the uptime against the allowed threshold value
	var maxThreshold int

	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		return fmt.Errorf("couldn't get image from metadata")
	}

	for family, threshold := range imageFamilyBootTimeThresholdMap {
		if strings.Contains(image, family) {
			maxThreshold = threshold
			break
		}
	}
	if maxThreshold == 0 {
		t.Log("using default boot time limit of 60s")
		maxThreshold = 60
	}

	if int(uptime) > maxThreshold {
		return fmt.Errorf("Boot time too long: %v is beyond max of %v", uptime, maxThreshold)
	}

	return nil
}

func verifyBootTimeWindows() error {
	command := `$boot = Get-WmiObject win32_operatingsystem
	$uptime = (Get-Date) - $boot.ConvertToDateTime($boot.LastBootUpTime)
	$uptime.TotalSeconds`
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		return fmt.Errorf("Failed to read uptime value")
	}
	uptimeString := strings.TrimSpace(output.Stdout)
	uptime, err := strconv.ParseFloat(uptimeString, 64)
	if err != nil {
		return fmt.Errorf("Failed to convert output to Integer: %v", err)
	}

	fmt.Println("found guest agent running at ", uptime, " seconds")

	var maxThreshold float64
	maxThreshold = 300
	if uptime > maxThreshold {
		return fmt.Errorf("Boot time too long: %v is beyond max of %v", uptime, maxThreshold)
	}

	return nil
}

func TestGuestBoot(t *testing.T) {
	t.Log("Guest booted successfully")
}

func TestGuestReboot(t *testing.T) {
	_, err := os.Stat(markerFile)
	if os.IsNotExist(err) {
		// first boot
		if _, err := os.Create(markerFile); err != nil {
			t.Fatalf("failed creating marker file: %v", err)
		}
	} else if err != nil {
		t.Fatalf("failed to stat marker file: %+v", err)
	}
	// second boot
	t.Log("marker file exist signal the guest reboot successful")
}

func TestGuestRebootOnHost(t *testing.T) {
	_, err := os.Stat(markerFile)
	if os.IsNotExist(err) {
		// first boot
		if _, err := os.Create(markerFile); err != nil {
			t.Fatalf("failed creating marker file: %v", err)
		}
		var cmd *exec.Cmd
		if utils.IsWindows() {
			cmd = exec.Command("shutdown", "-r", "-t", "0")
		} else {
			cmd = exec.Command("sudo", "nohup", "reboot")
		}
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to run reboot command: %v", err)
		}
		t.Fatal("marker file does not exist")
	}
	// second boot
	t.Log("marker file exist signal the guest reboot successful")
}

func TestGuestSecureBoot(t *testing.T) {
	if utils.IsWindows() {
		if err := testWindowsGuestSecureBoot(); err != nil {
			t.Fatalf("SecureBoot test failed with: %v", err)
		}
	} else {
		if err := testLinuxGuestSecureBoot(); err != nil {
			t.Fatalf("SecureBoot test failed with: %v", err)
		}
	}
}

func testLinuxGuestSecureBoot() error {
	if _, err := os.Stat(secureBootFile); os.IsNotExist(err) {
		return errors.New("secureboot efi var is missing")
	}
	data, err := ioutil.ReadFile(secureBootFile)
	if err != nil {
		return errors.New("failed reading secure boot file")
	}
	// https://www.kernel.org/doc/Documentation/ABI/stable/sysfs-firmware-efi-vars
	secureBootMode := data[len(data)-1]
	// https://uefi.org/specs/UEFI/2.9_A/32_Secure_Boot_and_Driver_Signing.html#firmware-os-key-exchange-creating-trust-relationships
	// If setup mode is not 0 secure boot isn't actually enabled because no PK is enrolled.
	if _, err = os.Stat(setupModeFile); os.IsNotExist(err) {
		return errors.New("setupmode efi var is missing")
	}
	data, err = ioutil.ReadFile(setupModeFile)
	if err != nil {
		return errors.New("failed reading setup mode file")
	}
	setupMode := data[len(data)-1]
	if secureBootMode != 1 || setupMode != 0 {
		return fmt.Errorf("secure boot is not enabled, found secureboot mode: %c (want 1) and setup mode: %c (want 0)", secureBootMode, setupMode)
	}
	return nil
}

func testWindowsGuestSecureBoot() error {
	cmd := exec.Command("powershell.exe", "Confirm-SecureBootUEFI")

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to run SecureBoot command: %v", err)
	}

	// The output will return a string that is either 'True' or 'False'
	// so we need to parse it and compare here.
	if trimmedOutput := strings.TrimSpace(string(output)); trimmedOutput != "True" {
		return errors.New("Secure boot is not enabled as expected")
	}

	return nil
}

func TestStartTime(t *testing.T) {
	metadata, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "start-time")
	if err != nil {
		t.Fatalf("couldn't get start time from metadata")
	}
	startTime, err := strconv.Atoi(metadata)
	if err != nil {
		t.Fatalf("failed to convet start time %s", metadata)
	}
	t.Logf("image start time is %d", time.Now().Second()-startTime)
}

func TestBootTime(t *testing.T) {

	var foundPassCondition bool

	// 300 is the current maximum number of seconds to allow any distro to start sshd and guest-agent before returning a test failure
	for i := 0; i < 300; i++ {
		if utils.IsWindows() {
			if err := lookForGuestAgentProcessWindows(); err == nil {
				foundPassCondition = true
				break
			}
		} else if err := lookForSshdAndGuestAgentProcess(); err == nil {
			foundPassCondition = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !foundPassCondition {
		t.Fatalf("Condition for guest agent and/or sshd process to start not reached within timeout")
	}

	if utils.IsWindows() {
		err := verifyBootTimeWindows()
		if err != nil {
			t.Fatalf("Failed to verify boot time: %v", err)
		}
	} else {
		err := verifyBootTime(t)
		if err != nil {
			t.Fatalf("Failed to verify boot time: %v", err)
		}
	}
}
