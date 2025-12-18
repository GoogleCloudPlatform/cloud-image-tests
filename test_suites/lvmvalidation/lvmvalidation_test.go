// Copyright 2025 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package lvmvalidation

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func runCmd(t *testing.T, command string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command(command, args...)
	stdout, err := cmd.Output()
	if err != nil {
		stderr := []byte{}
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = exitErr.Stderr
		}
		return string(stdout), string(stderr), fmt.Errorf("command %s %v failed: %w, stderr: %s", command, args, err, string(stderr))
	}
	return string(stdout), "", nil
}

// getSAPStatus checks if the image name indicates a RHEL for SAP image.
func getSAPStatus(ctx context.Context) (bool, error) {
	imageName, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		return false, fmt.Errorf("failed to get image name: %v", err)
	}
	return strings.Contains(imageName, "sap"), nil
}

// getLVMStatus returns true if the image is LVM, false otherwise.
func getLVMStatus(ctx context.Context) (bool, error) {
	imageName, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		return false, fmt.Errorf("failed to get image name: %v", err)
	}
	return strings.Contains(imageName, "lvm"), nil
}

// TestLVMPackage checks the lvm2 package install status.
// If the image is LVM, the lvm2 package should be installed.
// If the image is not LVM, the lvm2 package should not be installed.
func TestLVMPackage(t *testing.T) {
	isLVM, err := getLVMStatus(utils.Context(t))
	isSAP, err := getSAPStatus(utils.Context(t))
	if err != nil {
		t.Fatalf("Failed to get LVM status: %v", err)
	}

	// Check lvm2 package install status
	_, _, err = runCmd(t, "rpm", "-q", "lvm2")
	lvm2Installed := err == nil
	if isLVM || isSAP {
		if !lvm2Installed {
			t.Errorf("lvm2 package should be installed on LVM or SAP image, but it's not.")
		} else {
			t.Logf("lvm2 package is correctly installed on LVM or SAP image.")
		}
	} else { // Non-LVM expected
		if lvm2Installed {
			t.Errorf("lvm2 package should NOT be installed on non-LVM image, but it is.")
		} else {
			t.Logf("lvm2 package is correctly not installed on non-LVM image.")
		}
	}
}

func TestLVMExists(t *testing.T) {
	if utils.IsWindows() {
		t.Skip("LVM validation test only supports Linux images.")
	}

	isLVM, err := getLVMStatus(utils.Context(t))
	if err != nil {
		t.Fatalf("Failed to get LVM status: %v", err)
	}

	stdout, _, err := runCmd(t, "findmnt", "-n", "-o", "SOURCE", "/")
	if err != nil {
		t.Fatalf("Failed to get source for root filesystem: %v", err)
	}
	rootSource := strings.TrimSpace(stdout)
	isRootOnLVM := strings.HasPrefix(rootSource, "/dev/mapper/")

	if isLVM {
		if !isRootOnLVM {
			t.Errorf("Expected root on /dev/mapper/, but got %s", rootSource)
		} else {
			// Root is on LVM, now run the layout checks
			TestLVMLayout(t)
		}
	} else {
		if isRootOnLVM {
			t.Errorf("Root on non-LVM image should NOT be on /dev/mapper/, but got %s", rootSource)
		} else if !strings.HasPrefix(rootSource, "/dev/") {
			t.Errorf("Root source does not look like a device: %s", rootSource)
		}
	}
}

// TestLVMLayout checks the LVM layout.
// It checks for the existence of the LVM PV, Volume Group, and specific LVs.
func TestLVMLayout(t *testing.T) {
	t.Log("Checking LVM Layout...")

	// 1. Check for LVM PV
	stdout, _, err := runCmd(t, "lsblk", "-f", "-n", "-o", "FSTYPE")
	if err != nil || !strings.Contains(stdout, "LVM2_member") {
		t.Errorf("LVM PV partition type 'LVM2_member' not found")
	}

	// 2. Check for Volume Group
	if _, stderr, err := runCmd(t, "sudo", "vgs", "rootvg"); err != nil {
		t.Errorf("VG 'rootvg' not found: %v, %s", err, stderr)
		return // Stop here if VG is missing
	}

	// 3. Check for specific LVs
	expectedLVs := []string{"rootlv", "usrlv", "varlv", "tmplv", "homelv"}
	lvOutput, _, err := runCmd(t, "sudo", "lvs", "--noheadings", "-o", "lv_name", "rootvg")
	if err != nil {
		t.Fatalf("Failed to list LVs in rootvg: %v", err)
	}

	foundLVs := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(lvOutput))
	for scanner.Scan() {
		foundLVs[strings.TrimSpace(scanner.Text())] = true
	}

	for _, name := range expectedLVs {
		if !foundLVs[name] {
			t.Errorf("Expected Logical Volume '%s' not found in VG 'rootvg'", name)
		} else {
			t.Logf("Found LV: %s", name)
		}
	}
}
