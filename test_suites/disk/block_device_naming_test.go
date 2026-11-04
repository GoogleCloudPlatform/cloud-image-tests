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

package disk

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

var googleNVMeIDCandidatePaths = []string{
	"/usr/lib/udev/google_nvme_id",
	"/lib/udev/google_nvme_id",
}

const (
	nvmeCLIPath = "/usr/sbin/nvme"
	pdIDCtrl    = "mn : nvme_card-pd\nsn : nvme_card-pd\n"
	lssdIDCtrl  = "mn : nvme_card\nsn : local_ssd_0\n"
)

func findGoogleNVMeIDScript() (string, error) {
	for _, p := range googleNVMeIDCandidatePaths {
		if utils.Exists(p, utils.TypeFile) {
			return p, nil
		}
	}
	return "", fmt.Errorf("google_nvme_id not found in %v", googleNVMeIDCandidatePaths)
}

func queryUdevProperties(t *testing.T, devPath string) map[string]string {
	t.Helper()
	out, err := exec.Command("udevadm", "info", "--query=property", "--name="+devPath).CombinedOutput()
	if err != nil {
		t.Fatalf("udevadm info --query=property --name=%s failed: %v, output: %s", devPath, err, string(out))
	}
	props := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			props[k] = v
		}
	}
	return props
}

func TestBlockDeviceNaming(t *testing.T) {
	utils.LinuxOnly(t)
	if out, err := exec.Command("udevadm", "trigger").CombinedOutput(); err != nil {
		t.Fatalf("udevadm trigger failed: %v, output: %s", err, string(out))
	}
	if out, err := exec.Command("udevadm", "settle").CombinedOutput(); err != nil {
		t.Fatalf("udevadm settle failed: %v, output: %s", err, string(out))
	}
	disks, err := os.ReadDir("/dev/disk/by-id")
	if err != nil {
		t.Fatal(err)
	}
	var disklist []string
	for _, disk := range disks {
		disklist = append(disklist, disk.Name())
	}

	secondarySymlink := filepath.Join("/dev/disk/by-id", "google-"+secondaryDiskName)
	secondaryDev, err := filepath.EvalSymlinks(secondarySymlink)
	if err != nil {
		t.Fatalf("could not resolve %s (%v), found disks: %s", secondarySymlink, err, strings.Join(disklist, " "))
	}

	customSymlink := filepath.Join("/dev/disk/by-id", "google-"+customDeviceName)
	customDev, err := filepath.EvalSymlinks(customSymlink)
	if err != nil {
		t.Fatalf("could not resolve %s (%v), found disks: %s", customSymlink, err, strings.Join(disklist, " "))
	}
	if secondaryDev == customDev {
		t.Fatalf("%s and %s resolved to the same block device %s", secondarySymlink, customSymlink, secondaryDev)
	}

	props := queryUdevProperties(t, customDev)
	if got := props["ID_SERIAL_SHORT"]; got != customDeviceName {
		t.Errorf("udev property ID_SERIAL_SHORT on %s = %q, want %q", customDev, got, customDeviceName)
	}
	wantSerial := "Google_PersistentDisk_" + customDeviceName
	if got := props["ID_SERIAL"]; got != wantSerial {
		t.Errorf("udev property ID_SERIAL on %s = %q, want %q", customDev, got, wantSerial)
	}
}

// TestGoogleNVMeIDVendorExtensionValidation verifies that the installed
// google_nvme_id script validates NVMe Identify Namespace vendor-extension
// device_name values against Compute Engine's AttachedDisk.deviceName schema
// and rejects malformed or multi-line injection payloads.
func TestGoogleNVMeIDVendorExtensionValidation(t *testing.T) {
	utils.LinuxOnly(t)

	scriptPath, err := findGoogleNVMeIDScript()
	if err != nil {
		t.Fatalf("could not find google_nvme_id script: %v", err)
	}
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", scriptPath, err)
	}
	if !strings.Contains(string(scriptBytes), "DEVICE_NAME_REGEX") {
		t.Skipf("%s does not yet enforce DEVICE_NAME_REGEX (older guest-configs package); skipping vendor-extension validation test", scriptPath)
	}
	if !utils.Exists(nvmeCLIPath, utils.TypeFile) {
		t.Skipf("%s not present on image; skipping vendor-extension validation test", nvmeCLIPath)
	}
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skipf("unshare command not available (%v); skipping vendor-extension validation test", err)
	}

	tempDir := t.TempDir()
	mockNVMePath := filepath.Join(tempDir, "mock_nvme.sh")
	mockNVMeScript := `#!/bin/bash
set -eu
case "${1:-}" in
  id-ctrl)
    cat "${MOCK_NVME_ID_CTRL_FILE}"
    ;;
  id-ns)
    head -c 384 /dev/zero
    cat "${MOCK_NVME_VENDOR_EXT_FILE}"
    ;;
  *)
    exit 1
    ;;
esac
`
	if err := os.WriteFile(mockNVMePath, []byte(mockNVMeScript), 0755); err != nil {
		t.Fatalf("failed to write mock nvme script: %v", err)
	}

	runGoogleNVMeID := func(t *testing.T, idCtrl, vendorExt string, args ...string) (string, string, error) {
		t.Helper()
		caseDir := t.TempDir()
		mountDir := filepath.Join(caseDir, "mnt")
		if err := os.MkdirAll(mountDir, 0755); err != nil {
			t.Fatalf("failed to create mount dir: %v", err)
		}
		idCtrlFile := filepath.Join(caseDir, "id_ctrl.txt")
		if err := os.WriteFile(idCtrlFile, []byte(idCtrl), 0644); err != nil {
			t.Fatalf("failed to write id_ctrl fixture: %v", err)
		}
		vendorExtFile := filepath.Join(caseDir, "vendor_ext.bin")
		if err := os.WriteFile(vendorExtFile, []byte(vendorExt), 0644); err != nil {
			t.Fatalf("failed to write vendor_ext fixture: %v", err)
		}

		// Bind-mount the mock nvme binary from an executable tmpfs inside a
		// private mount namespace so /usr/sbin/nvme on the host VM is untouched
		// and noexec on /tmp cannot block execution.
		wrapper := `set -eu
mount --make-rprivate /
mount -t tmpfs -o size=1m,exec,mode=0755 tmpfs "${MOCK_NVME_MNT_DIR}"
cp "${MOCK_NVME_SRC}" "${MOCK_NVME_MNT_DIR}/nvme"
chmod 0755 "${MOCK_NVME_MNT_DIR}/nvme"
mount --bind "${MOCK_NVME_MNT_DIR}/nvme" /usr/sbin/nvme
exec "$@"`

		unshareArgs := append([]string{
			"-m",
			"/bin/bash", "-c", wrapper, "mock-nvme-wrapper",
			scriptPath,
		}, args...)
		cmd := exec.Command("unshare", unshareArgs...)
		cmd.Env = append(os.Environ(),
			"MOCK_NVME_MNT_DIR="+mountDir,
			"MOCK_NVME_SRC="+mockNVMePath,
			"MOCK_NVME_ID_CTRL_FILE="+idCtrlFile,
			"MOCK_NVME_VENDOR_EXT_FILE="+vendorExtFile,
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}

	t.Run("pd_valid_dotted_name", func(t *testing.T) {
		stdout, stderr, err := runGoogleNVMeID(t, pdIDCtrl, `{"device_name": "`+customDeviceName+`"}`, "-d", "/dev/nvme0n1")
		if err != nil {
			t.Fatalf("expected exit 0 for valid device_name, got err=%v, stderr=%q", err, stderr)
		}
		want := fmt.Sprintf("ID_SERIAL_SHORT=%s\nID_SERIAL=Google_PersistentDisk_%s\n", customDeviceName, customDeviceName)
		if stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})

	t.Run("pd_drops_unmatched_multiline_udev_env_injection", func(t *testing.T) {
		payload := "\"device_name\": \"valid_name\"\nLD_PRELOAD=/tmp/malicious.so device_name"
		stdout, stderr, err := runGoogleNVMeID(t, pdIDCtrl, payload, "-d", "/dev/nvme0n1")
		if err != nil {
			t.Fatalf("expected exit 0 when one valid device_name line matches, got err=%v, stderr=%q", err, stderr)
		}
		want := "ID_SERIAL_SHORT=valid_name\nID_SERIAL=Google_PersistentDisk_valid_name\n"
		if stdout != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
	})

	invalidCases := []struct {
		name      string
		vendorExt string
	}{
		{
			name:      "duplicate_device_name_lines",
			vendorExt: "{\"device_name\":\"disk1\"}\n{\"device_name\":\"disk2\"}",
		},
		{
			name:      "path_traversal_slashes",
			vendorExt: `{"device_name":"../../tmp/pwn"}`,
		},
		{
			name:      "path_traversal_dotdot",
			vendorExt: `{"device_name":".."}`,
		},
		{
			name:      "leading_dot",
			vendorExt: `{"device_name":".hidden"}`,
		},
		{
			name:      "leading_hyphen",
			vendorExt: `{"device_name":"-n"}`,
		},
		{
			name:      "empty_device_name",
			vendorExt: `{"device_name":""}`,
		},
		{
			name:      "overlong_256_chars",
			vendorExt: `{"device_name":"` + strings.Repeat("a", 256) + `"}`,
		},
	}

	for _, tc := range invalidCases {
		t.Run("pd_rejects_"+tc.name, func(t *testing.T) {
			stdout, stderr, err := runGoogleNVMeID(t, pdIDCtrl, tc.vendorExt, "-d", "/dev/nvme0n1")
			if err == nil {
				t.Fatalf("expected non-zero exit for payload %q, got exit 0 with stdout=%q stderr=%q", tc.vendorExt, stdout, stderr)
			}
			if stdout != "" {
				t.Errorf("expected empty stdout on rejected payload %q, got %q", tc.vendorExt, stdout)
			}
			if !strings.Contains(stderr, "Invalid device name") {
				t.Errorf("expected stderr to contain %q, got %q", "Invalid device name", stderr)
			}
		})
	}

	t.Run("lssd_vendor_ext_invalid_preserves_caller_name", func(t *testing.T) {
		stdout, stderr, err := runGoogleNVMeID(t, lssdIDCtrl, `{"device_name":"../../tmp/pwn"}`, "-e", "-d", "/dev/nvme0n1")
		if err != nil {
			t.Fatalf("expected -e to exit 0 on invalid vendor-extension name so udev keeps caller name, got err=%v, stderr=%q", err, stderr)
		}
		if stdout != "" {
			t.Errorf("expected empty stdout for -e on invalid vendor-extension name, got %q", stdout)
		}
	})
}
