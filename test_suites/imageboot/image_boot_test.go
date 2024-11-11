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
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// The values have been decided based on running spot tests for different images.
var imageBootTimeThresholds = []imageBootTimeThreshold{
	{Image: "almalinux", MaxTime: 20},
	{Image: "centos", MaxTime: 20},
	{Image: "debian", MaxTime: 20},
	{Image: "rhel", MaxTime: 30},
	{Image: "rocky-linux", MaxTime: 20},
	{Image: "opensuse", MaxTime: 40},
	{Image: "sles-12", MaxTime: 40},
	{Image: "sles-15", MaxTime: 40},
	{Image: "ubuntu", MaxTime: 30},
	{Image: "windows-11-", MaxTime: 200},
	{Image: "windows-server-2025", MaxTime: 200},
	{Image: "windows", MaxTime: 190},
}

type imageBootTimeThreshold struct {
	Image   string
	MaxTime int // In seconds
}

const (
	// See man 7 systemd.time
	systemdTimeFormat = "Mon 2006-01-02 15:04:05 MST"
	// https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.utility/get-uptime?view=powershell-7.4#example-2-show-the-time-of-the-last-boot
	windowsTimeFormat  = "Monday, January 2, 2006 3:04:05 PM"
	markerFile         = "/var/boot-marker"
	secureBootFile     = "/sys/firmware/efi/efivars/SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c"
	setupModeFile      = "/sys/firmware/efi/efivars/SetupMode-8be4df61-93ca-11d2-aa0d-00e098032b8c"
	defaultMaxBootTime = 20 // In seconds
)

func mountEFIVarsCOS(t *testing.T) error {
	t.Helper()

	if _, err := os.Stat(secureBootFile); !os.IsNotExist(err) {
		return nil
	}

	ctx := utils.Context(t)
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata %v", err)
	}

	if utils.IsCOS(image) {
		cmd := exec.CommandContext(ctx, "mount", "-t", "efivarfs", "efivarfs", "/sys/firmware/efi/efivars/")
		_, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("Failed to mount EFI vars: %v", err)
		}
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
		if err := os.MkdirAll(filepath.Dir(markerFile), 0755); err != nil {
			t.Fatalf("failed creating marker file directory: %v", err)
		}
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
		if err := os.MkdirAll(filepath.Dir(markerFile), 0755); err != nil {
			t.Fatalf("failed creating marker file directory: %v", err)
		}
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
		if err := testLinuxGuestSecureBoot(t); err != nil {
			t.Fatalf("SecureBoot test failed with: %v", err)
		}
	}
}

func testLinuxGuestSecureBoot(t *testing.T) error {
	if err := mountEFIVarsCOS(t); err != nil {
		return err
	}

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

func TestBootTime(t *testing.T) {
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Errorf("utils.GetMetadata(ctx, instance, image) = err %v want nil", err)
	}
	startTime := findInstanceStartTime(ctx, t)
	essentialServiceStartTime := findEssentialServiceStartTime(ctx, t, image)
	bootTime := int(essentialServiceStartTime.Sub(startTime).Seconds())
	t.Logf("Instance start time: %s", startTime.Format(time.ANSIC))
	t.Logf("Service start time: %s", essentialServiceStartTime.Format(time.ANSIC))
	if bootTime < 0 {
		t.Fatalf("Invalid boot time, services started before boot.")
	}
	maxBootTime := defaultMaxBootTime
	for _, threshold := range imageBootTimeThresholds {
		if strings.Contains(image, threshold.Image) {
			maxBootTime = threshold.MaxTime
			break
		}
	}
	if bootTime > maxBootTime {
		t.Errorf("Boot time of %d is greater than limit of %d", bootTime, maxBootTime)
	}
	if bootTime+10 < maxBootTime {
		t.Logf("Boot time of %d is more than 10 seconds below limit of %d. Consider lowering the limit if this is consistent.", bootTime, maxBootTime)
	}
}

func findInstanceStartTime(ctx context.Context, t *testing.T) time.Time {
	t.Helper()
	if utils.IsWindows() {
		cmd := "(Get-CimInstance Win32_OperatingSystem).LastBootUpTime"
		output, err := utils.RunPowershellCmd(cmd)
		if err != nil {
			t.Fatalf("utils.RunPowershellCmd(%s) = stderr: %v err: %v want err: nil", cmd, output.Stderr, err)
		}
		timestamp := strings.TrimSpace(output.Stdout)
		instanceStartTime, err := time.Parse(windowsTimeFormat, timestamp)
		if err != nil {
			t.Fatalf("time.Parse(windowsTimeFormat, %s) = %v want nil", timestamp, err)
		}
		return instanceStartTime
	}
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		t.Fatalf("os.ReadFile(/proc/uptime) = %v want nil", err)
	}
	fields := strings.Split(string(uptimeData), " ")
	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		t.Fatalf("strconv.ParseFloat(%s, 64) = %v want nil", fields, err)
	}
	instanceStartTime := time.Now().Add(time.Duration(-1*uptime) * time.Second)
	return instanceStartTime
}

// Find the time at which all essential services have been started. The list of
// essential services is decided from the image name.
func findEssentialServiceStartTime(ctx context.Context, t *testing.T, image string) time.Time {
	t.Helper()
	essentialServices := []string{"google-guest-agent.service", "sshd.service"}
	if strings.Contains(image, "windows") {
		essentialServices = []string{"GCEAgent"}
	} else if strings.Contains(image, "ubuntu") {
		essentialServices = []string{"google-guest-agent.service", "ssh.service"}
	}
	latestStartTime := time.Time{}
	for _, svc := range essentialServices {
		svcStart := findServiceStartTime(ctx, t, svc)
		if svcStart.After(latestStartTime) {
			latestStartTime = svcStart
		}
	}
	return latestStartTime
}

// Calling this function before the service is started will wait for it to be
// started before returning the start time.
func findServiceStartTime(ctx context.Context, t *testing.T, service string) time.Time {
	t.Helper()
	if utils.IsWindows() {
		for {
			output, err := utils.RunPowershellCmd(fmt.Sprintf(`(Get-Service -Name "%s").Status`, service))
			if err != nil {
				t.Fatalf("utils.RunPowershellCmd((Get-Service -Name %q).Status) = stderr: %v err: %v want err: nil", service, output.Stderr, err)
			}
			if strings.Contains(output.Stdout, "Running") {
				break
			}
			time.Sleep(time.Second)
			if ctx.Err() != nil {
				t.Fatalf("context expired before service %s was started: %v", service, ctx.Err())
			}
		}
		cmd := fmt.Sprintf(`(Get-Process -Id ((Get-CimInstance -ClassName Win32_Service | Where-Object {$_.Name -eq "%s"}).ProcessId)).StartTime`, service)
		output, err := utils.RunPowershellCmd(cmd)
		if err != nil {
			t.Fatalf("utils.RunPowershellCmd(%s) = stderr: %v err: %v want err: nil", cmd, output.Stderr, err)
		}
		timestamp := strings.TrimSpace(output.Stdout)
		serviceStartTime, err := time.Parse(windowsTimeFormat, timestamp)
		if err != nil {
			t.Fatalf("time.Parse(windowsServiceTimeFormat, %q) = %v want nil", timestamp, err)
		}
		return serviceStartTime
	}
	for {
		cmd := exec.CommandContext(ctx, "systemctl", "show", "--property=ActiveState", service)
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("exec.CommandContext(ctx, %s) = %v want nil", cmd.String(), err)
		}
		if strings.Contains(string(output), "ActiveState=active") {
			break
		}
		time.Sleep(time.Second)
		if ctx.Err() != nil {
			t.Fatalf("context expired before service %s was started: %v", service, ctx.Err())
		}
	}
	cmd := exec.CommandContext(ctx, "systemctl", "show", "--property=ActiveEnterTimestamp", service)
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, %s) = %v want nil", cmd.String(), err)
	}
	timestamp := strings.TrimPrefix(strings.TrimSpace(string(output)), "ActiveEnterTimestamp=")
	serviceStartTime, err := time.Parse(systemdTimeFormat, timestamp)
	if err != nil {
		t.Fatalf("time.Parse(systemdTimeFormat, %q) = %v want nil", timestamp, err)
	}
	return serviceStartTime
}
