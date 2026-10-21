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

package lssd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

var (
	udevRulesCandidatePaths = []string{
		"/usr/lib/udev/rules.d/65-gce-disk-naming.rules",
		"/lib/udev/rules.d/65-gce-disk-naming.rules",
	}
	googleNVMeIDCandidatePaths = []string{
		"/usr/lib/udev/google_nvme_id",
		"/lib/udev/google_nvme_id",
	}
	nvmeNamespaceDevRegex = regexp.MustCompile(`^(nvme[0-9]+)n[0-9]+$`)
)

// bareMetalSerialLSSDPCIID is the PCI vendor ID (0x1344) for Bare Metal NVMe
// Local SSDs named by serial ordering (such as on A5X Metal).
const bareMetalSerialLSSDPCIID = 0x1344

// nvmeLSSDDevice represents an attached NVMe Local SSD controller and namespace
// discovered via nvme-cli.
type nvmeLSSDDevice struct {
	controller string
	namespace  string
	devPath    string
	model      string
	serial     string
	vid        int
}

func getWindowsDiskNumber(ctx context.Context) (int, error) {
	diskName, err := utils.GetMetadata(ctx, "instance", "attributes", "hotattach-disk-name")
	if err != nil {
		return 0, err
	}
	intMatch := regexp.MustCompile("[0-9]+")
	o, err := utils.RunPowershellCmd(`(Get-Disk -FriendlyName "` + diskName + `").Number`)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(intMatch.FindString(o.Stdout))
}

func getLinuxMountPath(ctx context.Context) (string, error) {
	diskName, err := utils.GetMetadata(ctx, "instance", "attributes", "hotattach-disk-name")
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks("/dev/disk/by-id/google-" + diskName)
}

func mountLinuxDiskToPath(ctx context.Context, mountDiskDir string, isReattach bool) error {
	if err := os.MkdirAll(mountDiskDir, 0777); err != nil {
		return fmt.Errorf("could not make mount disk dir %s: error %v", mountDiskDir, err)
	}
	mountDiskPath, err := getLinuxMountPath(ctx)
	if err != nil {
		return err
	}
	if !utils.CheckLinuxCmdExists(mkfsCmd) {
		return fmt.Errorf("could not format mount disk: %s cmd not found", mkfsCmd)
	}
	if !isReattach {
		mkfsFullCmd := exec.Command(mkfsCmd, "-m", "0", "-E", "lazy_itable_init=0,lazy_journal_init=0,discard", "-F", mountDiskPath)
		if stdout, err := mkfsFullCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("mkfs cmd failed to complete: %v %v", stdout, err)
		}
	}

	mountCmd := exec.Command("mount", "-o", "discard,defaults", mountDiskPath, mountDiskDir)

	if err := mountCmd.Run(); err != nil {
		return fmt.Errorf("failed to mount disk: %v", err)
	}

	return nil
}

// TestMount tests that a drive can be mounted and written to. Hotattach without the attaching and detaching.
func TestMount(t *testing.T) {
	ctx := utils.Context(t)
	fileName := "hotattach.txt"
	fileContents := "cold Attach"
	fileContentsBytes := []byte(fileContents)
	var fileFullPath string
	if runtime.GOOS == "windows" {
		diskNum, err := getWindowsDiskNumber(ctx)
		if err != nil {
			diskNum = 1
		}
		procStatus, err := utils.RunPowershellCmd(fmt.Sprintf(`Initialize-Disk -PartitionStyle GPT -Number %d -PassThru | New-Partition -DriveLetter %s -UseMaximumSize | Format-Volume -FileSystem NTFS -NewFileSystemLabel 'Attach-Test' -Confirm:$false`, diskNum, windowsMountDriveLetter))
		if err != nil {
			t.Fatalf("failed to initialize disk on windows: errors %v, %s, %s", err, procStatus.Stdout, procStatus.Stderr)
		}
		fileFullPath = windowsMountDriveLetter + ":\\" + fileName
	} else {
		if err := mountLinuxDiskToPath(ctx, linuxMountPath, false); err != nil {
			t.Fatalf("failed to mount linux disk to linuxmountpath %s: error %v", linuxMountPath, err)
		}
		t.Cleanup(func() {
			_ = exec.Command("umount", linuxMountPath).Run()
		})
		fileFullPath = linuxMountPath + "/" + fileName
	}
	f, err := os.Create(fileFullPath)
	if err != nil {
		t.Fatalf("failed to create file at path %s: error %v", fileFullPath, err)
	}

	w := bufio.NewWriter(f)
	_, err = w.Write(fileContentsBytes)
	if err != nil {
		f.Close()
		t.Fatalf("failed to write bytes: err %v", err)
	}
	w.Flush()
	f.Sync()
	f.Close()

	hotAttachFile, err := os.Open(fileFullPath)
	if err != nil {
		t.Fatalf("file could not be reopened at path %s: error A%v", fileFullPath, err)
	}
	defer hotAttachFile.Close()

	fileLength, err := hotAttachFile.Read(fileContentsBytes)
	if fileLength == 0 {
		t.Fatalf("file was empty after writing to it")
	}
	if err != nil {
		t.Fatalf("reading file after writing failed with error: %v", err)
	}
}

func findFirstExistingFile(candidates []string) (string, error) {
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("none of %v exist", candidates)
}

func triggerAndSettleUdev(t *testing.T) {
	t.Helper()
	if out, err := exec.Command("udevadm", "trigger", "--subsystem-match=block").CombinedOutput(); err != nil {
		t.Fatalf("udevadm trigger failed: %v, output: %s", err, string(out))
	}
	if out, err := exec.Command("udevadm", "settle").CombinedOutput(); err != nil {
		t.Fatalf("udevadm settle failed: %v, output: %s", err, string(out))
	}
}

func discoverNVMeLSSDDevices(t *testing.T) []nvmeLSSDDevice {
	t.Helper()
	entries, err := os.ReadDir("/dev")
	if err != nil {
		t.Fatalf("failed to read /dev: %v", err)
	}

	var devices []nvmeLSSDDevice
	for _, entry := range entries {
		matches := nvmeNamespaceDevRegex.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		devPath := filepath.Join("/dev", entry.Name())
		out, err := exec.Command("nvme", "id-ctrl", "-o", "json", devPath).Output()
		if err != nil {
			t.Fatalf("nvme id-ctrl -o json %s failed: %v", devPath, err)
		}
		var idCtrl struct {
			VID int    `json:"vid"`
			MN  string `json:"mn"`
			SN  string `json:"sn"`
		}
		if err := json.Unmarshal(out, &idCtrl); err != nil {
			t.Fatalf("failed to unmarshal nvme id-ctrl JSON for %s: %v", devPath, err)
		}
		model := strings.TrimSpace(idCtrl.MN)
		// Virtualized and vendor-extension Bare Metal Local SSD controllers have
		// model "nvme_card" or "nvme_card<N>" (Persistent Disks have model
		// "nvme_card-pd"). Serial-ordered Bare Metal Local SSDs (e.g. A5X Metal)
		// have PCI vendor ID 0x1344 and a controller model that does not start
		// with "nvme_card".
		isGoogleLSSD := strings.HasPrefix(model, "nvme_card") && model != "nvme_card-pd"
		isBareMetalSerialLSSD := !strings.HasPrefix(model, "nvme_card") && idCtrl.VID == bareMetalSerialLSSDPCIID
		if !isGoogleLSSD && !isBareMetalSerialLSSD {
			continue
		}
		serial := strings.TrimSpace(idCtrl.SN)
		devices = append(devices, nvmeLSSDDevice{
			controller: matches[1],
			namespace:  entry.Name(),
			devPath:    devPath,
			model:      model,
			serial:     serial,
			vid:        idCtrl.VID,
		})
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].devPath < devices[j].devPath
	})
	return devices
}

func udevPropertiesForDevice(t *testing.T, devPath string) map[string]string {
	t.Helper()
	out, err := exec.Command("udevadm", "info", "--query=property", "--name="+devPath).CombinedOutput()
	if err != nil {
		t.Fatalf("udevadm info --query=property --name=%s failed: %v, output: %s", devPath, err, string(out))
	}
	props := make(map[string]string)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if k, v, ok := strings.Cut(line, "="); ok {
			props[k] = v
		}
	}
	return props
}

// googleNVMeIDFlagSupport returns the resolved google_nvme_id script path and
// whether its -h output documents the Bare Metal -e (vendor-extension) and -o
// (serial-ordering) flags.
// TODO(b/573992297): Remove legacy guest-configs fallback checks after NVMe
// Local SSD udev rules are fully rolled out across all guest images.
func googleNVMeIDFlagSupport(t *testing.T) (scriptPath string, supportsVendorExt, supportsSerialOrdering bool) {
	t.Helper()
	scriptPath, err := findFirstExistingFile(googleNVMeIDCandidatePaths)
	if err != nil {
		t.Fatalf("could not find google_nvme_id script: %v", err)
	}
	helpOut, err := exec.Command(scriptPath, "-h").CombinedOutput()
	if err != nil {
		t.Fatalf("%s -h failed: %v, output: %s", scriptPath, err, string(helpOut))
	}
	helpText := string(helpOut)
	return scriptPath, strings.Contains(helpText, "-e"), strings.Contains(helpText, "-o")
}

// skipIfBareMetalUdevUnsupported skips the calling Bare Metal test when the
// installed google_nvme_id script does not yet support the flag required for
// the attached Bare Metal Local SSD controller type (-e for nvme_card*
// controllers, or -o for serial-ordered controllers).
func skipIfBareMetalUdevUnsupported(t *testing.T) {
	t.Helper()
	utils.LinuxOnly(t)
	scriptPath, supportsVendorExt, supportsSerialOrdering := googleNVMeIDFlagSupport(t)
	devices := discoverNVMeLSSDDevices(t)
	if len(devices) > 0 && !strings.HasPrefix(devices[0].model, "nvme_card") {
		if !supportsSerialOrdering {
			t.Skipf("%s does not yet support -o flag (older guest-configs package); skipping Bare Metal LSSD test", scriptPath)
		}
		return
	}
	if !supportsVendorExt {
		t.Skipf("%s does not yet support -e flag (older guest-configs package); skipping Bare Metal LSSD test", scriptPath)
	}
}

// TestUdevRulesAndScript verifies that 65-gce-disk-naming.rules and
// google_nvme_id are present on the guest image and include the NVMe Local SSD
// rules (single-controller, multi-controller, partition symlinks, and Bare
// Metal vendor-extension / serial-ordering rules when supported by the
// installed google_nvme_id).
func TestUdevRulesAndScript(t *testing.T) {
	utils.LinuxOnly(t)

	scriptPath, supportsVendorExt, supportsSerialOrdering := googleNVMeIDFlagSupport(t)
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("failed to stat %s: %v", scriptPath, err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("%s is not executable (mode %v)", scriptPath, info.Mode())
	}
	if !supportsVendorExt {
		t.Logf("%s does not yet support -e flag; skipping Bare Metal vendor-extension udev rule check", scriptPath)
	}
	if !supportsSerialOrdering {
		t.Logf("%s does not yet support -o flag; skipping Bare Metal serial-ordering udev rule check", scriptPath)
	}

	rulesPath, err := findFirstExistingFile(udevRulesCandidatePaths)
	if err != nil {
		t.Fatalf("could not find 65-gce-disk-naming.rules: %v", err)
	}
	rulesContentBytes, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", rulesPath, err)
	}
	rulesContent := string(rulesContentBytes)

	expectedRuleSnippets := []string{
		`ATTRS{model}=="nvme_card"`,
		`ATTRS{model}=="nvme_card[0-9]*"`,
		`SYMLINK+="disk/by-id/google-$env{ID_SERIAL_SHORT}"`,
		`SYMLINK+="disk/by-id/google-$env{ID_SERIAL_SHORT}-part%n"`,
	}
	if supportsVendorExt {
		expectedRuleSnippets = append(expectedRuleSnippets,
			`ATTRS{model}=="nvme_card", ATTRS{serial}!="nvme_card*", IMPORT{program}="google_nvme_id -e -d $tempnode"`,
		)
	}
	if supportsSerialOrdering {
		expectedRuleSnippets = append(expectedRuleSnippets,
			`KERNEL=="nvme*", ATTRS{model}!="nvme_card*", GOTO="gce_bare_metal_serial_naming"`,
			`LABEL="gce_bare_metal_serial_naming"`,
			`KERNEL=="nvme*n*", ATTRS{vendor}=="0x1344", IMPORT{program}="google_nvme_id -o -d $tempnode"`,
			`KERNEL=="nvme*", ATTRS{vendor}=="0x1344", GOTO="gce_disk_naming_symlinks"`,
			`LABEL="gce_disk_naming_symlinks"`,
		)
	}
	for _, snippet := range expectedRuleSnippets {
		if !strings.Contains(rulesContent, snippet) {
			t.Errorf("%s is missing expected rule snippet %q", rulesPath, snippet)
		}
	}
}

// TestLSSDSymlinksAndUdevProperties verifies that every attached NVMe Local SSD
// has a unique /dev/disk/by-id/google-local-nvme-ssd-<N> symlink and matching
// ID_SERIAL_SHORT / ID_SERIAL udev properties after udevadm trigger and settle.
func TestLSSDSymlinksAndUdevProperties(t *testing.T) {
	utils.LinuxOnly(t)
	triggerAndSettleUdev(t)

	devices := discoverNVMeLSSDDevices(t)
	if len(devices) == 0 {
		t.Fatalf("expected at least 1 NVMe Local SSD in /sys/class/nvme, found 0")
	}

	seenTargets := make(map[string]string)
	for i := 0; i < len(devices); i++ {
		shortName := fmt.Sprintf("local-nvme-ssd-%d", i)
		symlinkPath := filepath.Join("/dev/disk/by-id", "google-"+shortName)

		targetPath, err := filepath.EvalSymlinks(symlinkPath)
		if err != nil {
			t.Fatalf("failed to resolve symlink %s: %v", symlinkPath, err)
		}
		if prevSymlink, exists := seenTargets[targetPath]; exists {
			t.Fatalf("symlink collision: both %s and %s resolve to %s", prevSymlink, symlinkPath, targetPath)
		}
		seenTargets[targetPath] = symlinkPath

		props := udevPropertiesForDevice(t, targetPath)
		if got := props["ID_SERIAL_SHORT"]; got != shortName {
			t.Errorf("udevadm property ID_SERIAL_SHORT for %s (%s) = %q, want %q", symlinkPath, targetPath, got, shortName)
		}
		wantSerial := "Google_EphemeralDisk_" + shortName
		if got := props["ID_SERIAL"]; got != wantSerial {
			t.Errorf("udevadm property ID_SERIAL for %s (%s) = %q, want %q", symlinkPath, targetPath, got, wantSerial)
		}
	}
}

// TestLSSDPartitionSymlinks verifies that creating a partition on an NVMe Local
// SSD produces the expected /dev/disk/by-id/google-local-nvme-ssd-<N>-part1
// symlink via 65-gce-disk-naming.rules.
func TestLSSDPartitionSymlinks(t *testing.T) {
	utils.LinuxOnly(t)
	if !utils.CheckLinuxCmdExists("sfdisk") {
		t.Skip("sfdisk not installed on guest image, skipping partition symlink test")
	}

	triggerAndSettleUdev(t)
	devices := discoverNVMeLSSDDevices(t)
	if len(devices) == 0 {
		t.Fatalf("expected at least 1 NVMe Local SSD, found 0")
	}

	// Prefer local-nvme-ssd-1 when multiple LSSDs are present so we do not touch
	// local-nvme-ssd-0 if it was formatted by TestMount.
	diskIndex := 0
	if len(devices) > 1 {
		diskIndex = 1
	} else {
		_ = exec.Command("umount", linuxMountPath).Run()
	}

	shortName := fmt.Sprintf("local-nvme-ssd-%d", diskIndex)
	diskSymlink := filepath.Join("/dev/disk/by-id", "google-"+shortName)
	targetDev, err := filepath.EvalSymlinks(diskSymlink)
	if err != nil {
		t.Fatalf("failed to resolve %s: %v", diskSymlink, err)
	}

	t.Cleanup(func() {
		_ = exec.Command("wipefs", "-a", targetDev).Run()
		_ = exec.Command("blockdev", "--rereadpt", targetDev).Run()
		_ = exec.Command("udevadm", "trigger", "--subsystem-match=block").Run()
		_ = exec.Command("udevadm", "settle").Run()
	})

	cmd := exec.Command("sfdisk", "--wipe=always", targetDev)
	cmd.Stdin = strings.NewReader("size=64M, type=linux\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sfdisk failed on %s: %v, output: %s", targetDev, err, string(out))
	}

	triggerAndSettleUdev(t)

	partSymlink := diskSymlink + "-part1"
	partTarget, err := filepath.EvalSymlinks(partSymlink)
	if err != nil {
		t.Fatalf("expected partition symlink %s to exist after partitioning %s: %v", partSymlink, targetDev, err)
	}
	wantPartTarget := targetDev + "p1"
	if partTarget != wantPartTarget {
		t.Errorf("partition symlink %s resolved to %s, want %s", partSymlink, partTarget, wantPartTarget)
	}
}

func verifyGoogleNVMeIDOutput(t *testing.T, useVendorExtension bool) {
	t.Helper()
	scriptPath, err := findFirstExistingFile(googleNVMeIDCandidatePaths)
	if err != nil {
		t.Fatalf("could not find google_nvme_id script: %v", err)
	}

	devices := discoverNVMeLSSDDevices(t)
	if len(devices) == 0 {
		t.Fatalf("expected at least 1 NVMe Local SSD, found 0")
	}

	seenNames := make(map[string]string)
	for _, dev := range devices {
		var args []string
		if useVendorExtension {
			if dev.model != "nvme_card" || strings.HasPrefix(dev.serial, "nvme_card") {
				t.Errorf("Bare Metal LSSD controller %s has model=%q serial=%q; expected model=\"nvme_card\" and serial without \"nvme_card\" prefix so the Bare Metal -e udev rule matches", dev.controller, dev.model, dev.serial)
			}
			args = []string{"-e", "-d", dev.devPath}
		} else {
			if !strings.HasPrefix(dev.serial, "nvme_card") {
				t.Errorf("VM LSSD controller %s (model %q) has unexpected serial %q; expected prefix \"nvme_card\" so the Bare Metal -e udev rule does not match", dev.controller, dev.model, dev.serial)
			}
			args = []string{"-d", dev.devPath}
		}

		out, err := exec.Command(scriptPath, args...).Output()
		if err != nil {
			t.Fatalf("%s %s failed (model=%q, serial=%q): %v", scriptPath, strings.Join(args, " "), dev.model, dev.serial, err)
		}
		props := make(map[string]string)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
				props[k] = v
			}
		}
		shortName := props["ID_SERIAL_SHORT"]
		if !strings.HasPrefix(shortName, "local-nvme-ssd-") {
			t.Fatalf("%s %s returned ID_SERIAL_SHORT=%q, want local-nvme-ssd-<N>", scriptPath, strings.Join(args, " "), shortName)
		}
		if prevDev, exists := seenNames[shortName]; exists {
			t.Fatalf("duplicate LSSD name %q on %s and %s", shortName, prevDev, dev.devPath)
		}
		seenNames[shortName] = dev.devPath

		wantSerial := "Google_EphemeralDisk_" + shortName
		if got := props["ID_SERIAL"]; got != wantSerial {
			t.Errorf("%s %s returned ID_SERIAL=%q, want %q", scriptPath, strings.Join(args, " "), got, wantSerial)
		}

		symlinkPath := filepath.Join("/dev/disk/by-id", "google-"+shortName)
		resolved, err := filepath.EvalSymlinks(symlinkPath)
		if err != nil {
			t.Errorf("failed to resolve symlink %s: %v", symlinkPath, err)
		} else if resolved != dev.devPath {
			t.Errorf("symlink %s resolved to %s, want %s", symlinkPath, resolved, dev.devPath)
		}
	}
}

// TestLSSDGoogleNVMeIDNaming verifies on VM instances that every LSSD
// controller serial starts with "nvme_card" (so the Bare Metal -e udev rule
// never matches on a VM) and that google_nvme_id -d (without -e) produces a
// unique local-nvme-ssd-<N> name matching its /dev/disk/by-id symlink.
func TestLSSDGoogleNVMeIDNaming(t *testing.T) {
	utils.LinuxOnly(t)
	verifyGoogleNVMeIDOutput(t, false)
}

// TestBareMetalMount runs TestMount on a Bare Metal LSSD instance.
func TestBareMetalMount(t *testing.T) {
	TestMount(t)
}

// TestBareMetalUdevRulesAndScript runs TestUdevRulesAndScript on a Bare Metal
// LSSD instance.
func TestBareMetalUdevRulesAndScript(t *testing.T) {
	TestUdevRulesAndScript(t)
}

// TestBareMetalLSSDSymlinksAndUdevProperties runs
// TestLSSDSymlinksAndUdevProperties on a Bare Metal LSSD instance.
func TestBareMetalLSSDSymlinksAndUdevProperties(t *testing.T) {
	skipIfBareMetalUdevUnsupported(t)
	TestLSSDSymlinksAndUdevProperties(t)
}

// TestBareMetalLSSDPartitionSymlinks runs TestLSSDPartitionSymlinks on a Bare
// Metal LSSD instance.
func TestBareMetalLSSDPartitionSymlinks(t *testing.T) {
	skipIfBareMetalUdevUnsupported(t)
	TestLSSDPartitionSymlinks(t)
}

// TestBareMetalLSSDVendorExtensionNaming verifies on Bare Metal LSSD instances
// whose controllers use the "nvme_card" model (ATTRS{model}=="nvme_card" and
// ATTRS{serial}!="nvme_card*", such as Z3 or C4D Metal) that google_nvme_id -e
// reads a unique device_name from the NVMe Identify Namespace vendor extension
// for each drive and that /dev/disk/by-id/google-<ID_SERIAL_SHORT> resolves to
// that drive.
func TestBareMetalLSSDVendorExtensionNaming(t *testing.T) {
	utils.LinuxOnly(t)
	devices := discoverNVMeLSSDDevices(t)
	if len(devices) == 0 {
		t.Fatalf("discoverNVMeLSSDDevices() returned 0 devices, want >= 1")
	}
	if !strings.HasPrefix(devices[0].model, "nvme_card") {
		t.Skipf("Bare Metal LSSD controller %s has model=%q (not nvme_card*); skipping vendor-extension (-e) naming test", devices[0].controller, devices[0].model)
	}
	skipIfBareMetalUdevUnsupported(t)
	verifyGoogleNVMeIDOutput(t, true)
}

// TestBareMetalLSSDSerialOrderingNaming verifies on Bare Metal LSSD instances
// using serial-ordered naming (ATTRS{model}!="nvme_card*" and
// ATTRS{vendor}=="0x1344", such as A5X Metal) that google_nvme_id -o assigns
// deterministic 0-based local-nvme-ssd-<N> names according to the
// lexicographical order of controller serial numbers, and that
// /dev/disk/by-id/google-local-nvme-ssd-<N> resolves to the corresponding drive.
func TestBareMetalLSSDSerialOrderingNaming(t *testing.T) {
	utils.LinuxOnly(t)
	devices := discoverNVMeLSSDDevices(t)
	if len(devices) == 0 {
		t.Fatalf("discoverNVMeLSSDDevices() returned 0 devices, want >= 1")
	}
	if strings.HasPrefix(devices[0].model, "nvme_card") {
		t.Skipf("Bare Metal LSSD controller %s has model=%q (nvme_card*); skipping serial-ordering (-o) naming test", devices[0].controller, devices[0].model)
	}
	skipIfBareMetalUdevUnsupported(t)
	scriptPath, _, _ := googleNVMeIDFlagSupport(t)

	for _, dev := range devices {
		if strings.HasPrefix(dev.model, "nvme_card") {
			t.Fatalf("controller %s model = %q, want non-nvme_card* model", dev.controller, dev.model)
		}
		if dev.vid != bareMetalSerialLSSDPCIID {
			t.Fatalf("controller %s vendor ID = 0x%x, want 0x%x", dev.controller, dev.vid, bareMetalSerialLSSDPCIID)
		}
		if dev.serial == "" {
			t.Fatalf("controller %s (%s) serial = %q, want non-empty serial", dev.controller, dev.devPath, dev.serial)
		}
		sysfsVendorPath := filepath.Join("/sys/class/nvme", dev.controller, "device/vendor")
		vendorBytes, err := os.ReadFile(sysfsVendorPath)
		if err != nil {
			t.Fatalf("failed to read %s: %v", sysfsVendorPath, err)
		}
		const wantVendor = "0x1344"
		if gotVendor := strings.ToLower(strings.TrimSpace(string(vendorBytes))); gotVendor != wantVendor {
			t.Errorf("os.ReadFile(%q) = %q, want %q", sysfsVendorPath, gotVendor, wantVendor)
		}
	}

	sortedBySerial := make([]nvmeLSSDDevice, len(devices))
	copy(sortedBySerial, devices)
	sort.Slice(sortedBySerial, func(i, j int) bool {
		return sortedBySerial[i].serial < sortedBySerial[j].serial
	})

	for i, dev := range sortedBySerial {
		if i > 0 && dev.serial == sortedBySerial[i-1].serial {
			t.Fatalf("duplicate controller serial number %q on %s and %s", dev.serial, sortedBySerial[i-1].devPath, dev.devPath)
		}

		wantShortName := fmt.Sprintf("local-nvme-ssd-%d", i)
		wantSerial := "Google_EphemeralDisk_" + wantShortName

		out, err := exec.Command(scriptPath, "-o", "-d", dev.devPath).Output()
		if err != nil {
			t.Fatalf("%s -o -d %s failed (model=%q, serial=%q): %v", scriptPath, dev.devPath, dev.model, dev.serial, err)
		}
		props := make(map[string]string)
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
				props[k] = v
			}
		}
		if got := props["ID_SERIAL_SHORT"]; got != wantShortName {
			t.Errorf("%s -o -d %s (serial=%q, rank=%d) returned ID_SERIAL_SHORT=%q, want %q", scriptPath, dev.devPath, dev.serial, i, got, wantShortName)
		}
		if got := props["ID_SERIAL"]; got != wantSerial {
			t.Errorf("%s -o -d %s (serial=%q, rank=%d) returned ID_SERIAL=%q, want %q", scriptPath, dev.devPath, dev.serial, i, got, wantSerial)
		}

		symlinkPath := filepath.Join("/dev/disk/by-id", "google-"+wantShortName)
		resolved, err := filepath.EvalSymlinks(symlinkPath)
		if err != nil {
			t.Errorf("failed to resolve symlink %s: %v", symlinkPath, err)
		} else if resolved != dev.devPath {
			t.Errorf("symlink %s resolved to %s, want %s (serial=%q, rank=%d)", symlinkPath, resolved, dev.devPath, dev.serial, i)
		}

		udevProps := udevPropertiesForDevice(t, dev.devPath)
		if got := udevProps["ID_SERIAL_SHORT"]; got != wantShortName {
			t.Errorf("udev property ID_SERIAL_SHORT for %s (serial=%q, rank=%d) = %q, want %q", dev.devPath, dev.serial, i, got, wantShortName)
		}
		if got := udevProps["ID_SERIAL"]; got != wantSerial {
			t.Errorf("udev property ID_SERIAL for %s (serial=%q, rank=%d) = %q, want %q", dev.devPath, dev.serial, i, got, wantSerial)
		}

		// Calling google_nvme_id -e on a drive that does not publish a vendor
		// extension must exit 0 and emit nothing.
		vendorExtOut, err := exec.Command(scriptPath, "-e", "-d", dev.devPath).Output()
		if err != nil {
			t.Errorf("%s -e -d %s failed: %v", scriptPath, dev.devPath, err)
		} else if trimmed := strings.TrimSpace(string(vendorExtOut)); trimmed != "" {
			t.Errorf("%s -e -d %s emitted %q on non-vendor-extension drive, want empty output", scriptPath, dev.devPath, trimmed)
		}
	}
}
