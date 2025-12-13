// Copyright 2024 Google LLC.
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

// getLVMStatus returns true if the image is LVM, false otherwise.
func getLVMStatus(ctx context.Context) (bool, error) {
	imageName, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		return false, fmt.Errorf("failed to get image name: %v", err)
	}
	return strings.Contains(imageName, "lvm"), nil
}

// TestLVMPackage checks the lvm2 package status.
// If the image is LVM, the lvm2 package should be installed.
// If the image is not LVM, the lvm2 package should not be installed.
func TestLVMPackage(t *testing.T) {
	if utils.IsWindows() {
		t.Skip("LVM validation test only supports Linux images.")
	}

	isLVM, err := getLVMStatus(utils.Context(t))
	if err != nil {
		t.Fatalf("Failed to get LVM status: %v", err)
	}

	// Check lvm2 package status
	_, _, err = runCmd(t, "rpm", "-q", "lvm2")
	lvm2Installed := err == nil
	if isLVM {
		if !lvm2Installed {
			t.Errorf("lvm2 package should be installed on LVM image, but it's not.")
		} else {
			t.Logf("lvm2 package is correctly installed on LVM image.")
		}
	} else { // Non-LVM expected
		if lvm2Installed {
			t.Errorf("lvm2 package should NOT be installed on non-LVM image, but it is.")
		} else {
			t.Logf("lvm2 package is correctly not installed on non-LVM image.")
		}
	}
}

// TestLVMLayout checks the root filesystem layout.
// If the image is LVM, the root filesystem should be on /dev/mapper/.
// If the image is not LVM, the root filesystem should be on /dev/ and not /dev/mapper/.
func TestLVMLayout(t *testing.T) {
	if utils.IsWindows() {
		t.Skip("LVM validation test only supports Linux images.")
	}

	isLVM, err := getLVMStatus(utils.Context(t))
	if err != nil {
		t.Fatalf("Failed to get LVM status: %v", err)
	}

	// Check if the root filesystem is on LVM
	stdout, _, err := runCmd(t, "findmnt", "-n", "-o", "SOURCE", "/")
	if err != nil {
		t.Fatalf("Failed to get source for root filesystem: %v", err)
	}
	rootSource := strings.TrimSpace(stdout)
	t.Logf("Root filesystem source: %s", rootSource)

	isRootOnLVM := strings.HasPrefix(rootSource, "/dev/mapper/")

	if isLVM {
		if !isRootOnLVM {
			t.Errorf("Root filesystem on LVM image should be on /dev/mapper/, but got %s", rootSource)
		} else {
			t.Logf("Root filesystem is on LVM as expected.")
			TestLVMExists(t)
		}
	} else { // Non-LVM expected
		if isRootOnLVM {
			t.Errorf("Root filesystem on non-LVM image should NOT be on /dev/mapper/, but got %s", rootSource)
		} else if !strings.HasPrefix(rootSource, "/dev/") {
			t.Errorf("Root filesystem source on non-LVM image does not look like a device: %s", rootSource)
		} else {
			t.Logf("Root filesystem is on a standard partition %s as expected.", rootSource)
		}
	}
}

func TestLVMExists(t *testing.T) {
	t.Logf("Checking for basic LVM Layout Existence...")

	// 1. Check for an LVM PV partition
	stdout, _, err := runCmd(t, "lsblk", "-f", "-n", "-o", "FSTYPE")
	if err != nil {
		t.Fatalf("Failed to run lsblk -f: %v", err)
	}
	if !strings.Contains(stdout, "LVM2_member") {
		t.Errorf("LVM PV partition type 'LVM2_member' not found in lsblk output")
	} else {
		t.Logf("LVM2_member partition found.")
	}

	// 2. Check root filesystem is on LVM
	rootSource, _, err := runCmd(t, "findmnt", "-n", "-o", "SOURCE", "/")
	if err != nil {
		t.Fatalf("Failed to get source for root filesystem: %v", err)
	}
	rootSource = strings.TrimSpace(rootSource)
	if !strings.HasPrefix(rootSource, "/dev/mapper/") {
		t.Errorf("Root filesystem should be on /dev/mapper/, but got %s", rootSource)
	} else {
		t.Logf("Root filesystem is on LVM: %s", rootSource)
	}

	// 3. Check for Volume Group 'rootvg'
	_, stderr, err := runCmd(t, "sudo", "vgs", "rootvg")
	if err != nil {
		t.Errorf("Volume Group 'rootvg' not found: %v, stderr: %s", err, stderr)
	} else {
		t.Logf("VG 'rootvg' found.")
	}

	// 4. Check for the existence of key Logical Volumes
	expectedLVNames := []string{"rootlv", "usrlv", "varlv", "tmplv", "homelv"}
	lvOutput, _, err := runCmd(t, "sudo", "lvs", "--noheadings", "-o", "lv_name", "rootvg")
	if err != nil {
		t.Fatalf("Failed to list LVs in rootvg: %v", err)
	}

	foundLVs := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(lvOutput))
	for scanner.Scan() {
		foundLVs[strings.TrimSpace(scanner.Text())] = true
	}

	for _, name := range expectedLVNames {
		if !foundLVs[name] {
			t.Errorf("Expected Logical Volume '%s' not found in VG 'rootvg'", name)
		} else {
			t.Logf("Found LV: %s", name)
		}
	}
}
