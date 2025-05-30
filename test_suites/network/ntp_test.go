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

package network

import (
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	chronyService    = "chronyd"
	ntpService       = "ntp"
	ntpdService      = "ntpd"
	chronycCmd       = "chronyc"
	ntpqCmd          = "ntpq"
	systemdTimesyncd = "systemd-timesyncd"
	timedatectlCmd   = "timedatectl"
)

func TestNTP(t *testing.T) {
	if runtime.GOOS == "windows" {
		testNTPWindows(t)
	} else {
		testNTPServiceLinux(t)
	}
}

// testNTPService Verify that ntp package exist and configuration is correct.
// debian 9, ubuntu 16.04 ntp
// debian 12 systemd-timesyncd
// sles-12 ntpd
// other distros chronyd
func testNTPServiceLinux(t *testing.T) {
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata")
	}
	var servicename string
	switch {
	case strings.Contains(image, "debian-12"), strings.Contains(image, "debian-13"):
		servicename = systemdTimesyncd
	case strings.Contains(image, "debian-9"), strings.Contains(image, "ubuntu-pro-1604"), strings.Contains(image, "ubuntu-1604"):
		servicename = ntpService
	case strings.Contains(image, "sles-12"):
		servicename = ntpdService
	default:
		servicename = chronyService
	}
	var cmd *exec.Cmd
	if utils.CheckLinuxCmdExists(chronycCmd) {
		cmd = exec.Command(chronycCmd, "-c", "sources")
	} else if utils.CheckLinuxCmdExists(ntpqCmd) {
		cmd = exec.Command(ntpqCmd, "-np")
	} else if utils.CheckLinuxCmdExists(timedatectlCmd) {
		cmd = exec.Command(timedatectlCmd, "show-timesync", "--property=FallbackNTPServers")
	} else {
		t.Fatalf("failed to find timedatectl chronyc or ntpq cmd")
	}

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("ntp command failed %v", err)
	}
	serverNames := []string{"metadata.google.internal", "metadata", "169.254.169.254"}
	foundNtpServer := false
	outputString := string(out)
	for _, serverName := range serverNames {
		if strings.Contains(outputString, serverName) {
			foundNtpServer = true
			break
		}
	}
	if !foundNtpServer {
		t.Fatalf("could not find ntp server")
	}

	// Make sure that ntp service is running.
	systemctlCmd := exec.Command("systemctl", "is-active", servicename)
	if err := systemctlCmd.Run(); err != nil {
		t.Fatalf("%s service is not running", servicename)
	}
}

func testNTPWindows(t *testing.T) {
	ensureNTPServiceRunning(t)
	command := "w32tm /query /peers /verbose"
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error getting NTP information: %v", err)
	}

	expected := []string{
		"#Peers: 1",
		"LastSyncErrorMsgId: 0x00000000 (Succeeded)",
	}

	for _, exp := range expected {
		if !strings.Contains(output.Stdout, exp) {
			t.Fatalf("Expected info %s not found in peer_info: %s", exp, output.Stdout)
		}
	}

	expectedAddresses := []string{
		"Peer: metadata.google.internal,0x1",
		"Peer: 169.254.169.254,0x1",
	}

	// At least one of the expected addresses should be present.
	if !strings.Contains(output.Stdout, expectedAddresses[0]) && !strings.Contains(output.Stdout, expectedAddresses[1]) {
		t.Fatalf("Expected address %s or %s not found in peer_addresses: %s", expectedAddresses[0], expectedAddresses[1], output.Stdout)
	}

	// NTP can take time to get to an active state.
	if !(strings.Contains(output.Stdout, "State: Active") || strings.Contains(output.Stdout, "State: Pending")) {
		t.Fatalf("Expected State: Active or Pending in: %s", output.Stdout)
	}

	r, err := regexp.Compile("Time Remaining: ([0-9\\.]+)s")
	if err != nil {
		t.Fatalf("Error creating regexp: %v", err)
	}

	remaining := r.FindStringSubmatch(output.Stdout)[1]
	remainingTime, err := strconv.ParseFloat(remaining, 32)
	if err != nil {
		t.Fatalf("Unexpected remaining time value: %s", remaining)
	}

	if remainingTime < 0.0 {
		t.Fatalf("Invalid remaining time: %f", remainingTime)
	}

	if remainingTime > 900.0 {
		t.Fatalf("Time remaining is longer than the 15 minute poll interval: %f", remainingTime)
	}
}

func ensureNTPServiceRunning(t *testing.T) {
	t.Helper()
	command := "(Get-Service w32time).Status"
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("RunPowershellCmd(%s) failed: %v", command, err)
	}
	if output.Stdout != "Running" {
		output, err = utils.RunPowershellCmd("Start-Service w32time")
		if err != nil {
			t.Fatalf("RunPowershellCmd(%s) failed: %v", "Start-Service w32time", err)
		}
	}
}
