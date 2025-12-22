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

package hostnamevalidation

import (
	"bufio"
	"crypto/md5"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/exceptions"
	// allowlist:crypto/md5
)

const gcomment = "# Added by Google"

func testHostnameWindows(shortname string) error {
	command := "[System.Net.Dns]::GetHostName()"
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		return fmt.Errorf("Error getting hostname: %v", err)
	}
	hostname := strings.TrimSpace(output.Stdout)

	if hostname != shortname {
		return fmt.Errorf("Expected Hostname: '%s', Actual Hostname: '%s'", shortname, hostname)
	}
	return nil
}

func testHostnameLinux(shortname string) error {
	hostnameBytes, err := exec.Command("/bin/hostname").Output()
	if err != nil {
		return fmt.Errorf("couldn't determine local hostname: %v", utils.ParseStderr(err))
	}
	hostname := strings.TrimSpace(string(hostnameBytes))

	if hostname != shortname {
		return fmt.Errorf("hostname does not match metadata. Expected: %q got: %q", shortname, hostname)
	}

	// If hostname is FQDN then lots of tools (e.g. ssh-keygen) have issues
	// However, if the expected hostname is FQDN, and the hostname is FQDN, then don't error.
	// TODO(b/434038215): Update this logic when/if hostname inconsistency is
	// resolved with Ubuntu.
	if strings.Contains(hostname, ".") != strings.Contains(shortname, ".") {
		return fmt.Errorf("hostname contains FQDN")
	}
	return nil
}

func TestHostname(t *testing.T) {
	ctx := utils.Context(t)
	metadataHostname, err := utils.GetMetadata(ctx, "instance", "hostname")
	if err != nil {
		t.Fatalf("still couldn't determine metadata hostname")
	}
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("couldn't get image from metadata: %v", err)
	}
	image = filepath.Base(image)

	// On Ubuntu versions >= 24.04 (but not minimal), the hostname is the FQDN.
	// However, if the hostname is longer than 63 characters, it will be truncated.
	// See https://man7.org/linux/man-pages/man7/hostname.7.html for more details.
	// On minimal images, the hostname is the shortname.
	hostnameExceptions := []exceptions.Exception{
		exceptions.Exception{Version: 2404, Type: exceptions.GreaterThanOrEqualTo},
	}

	// 'hostname' in metadata is fully qualified domain name.
	shortname := strings.Split(metadataHostname, ".")[0]
	if exceptions.MatchAll(image, exceptions.ImageUbuntuNoMinimal, hostnameExceptions...) {
		if len(metadataHostname) < 64 {
			shortname = metadataHostname
		} else {
			t.Logf("Hostname is longer than 63 characters, testing for shortname")
		}
	}

	if runtime.GOOS == "windows" {
		if err = testHostnameWindows(shortname); err != nil {
			t.Fatalf("windows hostname error: %v", err)
		}
	} else {
		if err = testHostnameLinux(shortname); err != nil {
			t.Fatalf("linux hostname error: %v", err)
		}
	}
}

// TestCustomHostname tests the 'fully qualified domain name'.
func TestCustomHostname(t *testing.T) {
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata")
	}

	// guest-configs does not support wicked
	if strings.Contains(image, "sles") {
		t.Skip("SLES doesn't support custom hostnames.")
	}
	if strings.Contains(image, "suse") {
		t.Skip("SUSE doesn't support custom hostnames.")
	}
	if strings.Contains(image, "cos") {
		// Does not have updated guest-configs with systemd-network support.
		t.Skip("Not supported on cos")
	}
	if strings.Contains(image, "ubuntu") {
		// Does not have updated guest-configs with systemd-network support.
		t.Skip("Not supported on ubuntu")
	}

	TestFQDN(t)
}

// TestFQDN tests the 'fully qualified domain name'.
func TestFQDN(t *testing.T) {
	utils.LinuxOnly(t)
	ctx := utils.Context(t)

	metadataHostname, err := utils.GetMetadata(ctx, "instance", "hostname")
	if err != nil {
		t.Fatalf("couldn't determine metadata hostname")
	}

	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("couldn't get image from metadata: %v", err)
	}

	// Get the hostname with FQDN.
	cmd := exec.Command("/bin/hostname", "-f")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hostname command failed: %v", utils.ParseStderr(err))
	}
	hostname := strings.TrimRight(string(out), " \n")

	printSuseDebugInfo(t)
	if hostname != metadataHostname {
		// TODO(b/460799853): Remove this exception once the bug is fixed.
		if strings.Contains(image, "sles") && strings.Contains(image, "arm") {
			t.Skipf("Skipping TestFQDN for SLES ARM image: %q due to b/460799853, got hostname: %q, want hostname: %q", image, hostname, metadataHostname)
		}
		t.Errorf("hostname -f does not match metadata. Expected: %q got: %q", metadataHostname, hostname)
	}
}

func printLeaseFiles(t *testing.T, dir, prefix string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Logf("Error reading directory %q: %v", dir, err)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if prefix != "" && !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		fullPath := filepath.Join(dir, entry.Name())
		t.Logf("--- Found: %s ---", fullPath)

		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Logf("Could not read file %s: %v", fullPath, err)
			continue
		}

		t.Log(string(content))
	}
}

func isSles(t *testing.T) bool {
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Logf("Error reading /etc/os-release: %v, defaulting isSLES to false", err)
		return false
	}
	return strings.Contains(string(content), "sles")
}

// printSuseDebugInfo prints the DHCP lease information as reported by wicked.
// This is used to help diagnose test failures on SLES as they're not
// reproducible outside testgrid runs.
func printSuseDebugInfo(t *testing.T) {
	if !isSles(t) {
		t.Logf("Not SLES image, skipping SUSE debug info")
		return
	}

	t.Logf("SUSE DHCP lease:")

	// SLE 12 and 15 use wicked, SLE 16 uses NetworkManager. wicked stores DHCP
	// leases in /var/lib/wicked/lease<id>. NetworkManager stores them in
	// /run/NetworkManager/devices/$IFINDEX.
	wickedDir := "/var/lib/wicked"
	nmDir := "/run/NetworkManager/devices"
	if utils.Exists(wickedDir, utils.TypeDir) {
		printLeaseFiles(t, wickedDir, "lease")
	} else if utils.Exists(nmDir, utils.TypeDir) {
		printLeaseFiles(t, nmDir, "")
	} else {
		t.Logf("No wicked (%s) or NetworkManager (%s) directory found, skipping SUSE DHCP lease debug info", wickedDir, nmDir)
	}

	etcResolv := "/etc/resolv.conf"
	content, err := os.ReadFile(etcResolv)
	if err != nil {
		t.Logf("Could not read file %s: %v", etcResolv, err)
	} else {
		t.Logf("Contents of %s:\n\n%s\n\n", etcResolv, string(content))
	}

	return
}

func md5Sum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("couldn't open file: %v", err)
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

type sshKeyHash struct {
	file os.FileInfo
	hash string
}

// TestHostKeysGeneratedOnces checks that the guest agent only generates host keys one time.
func TestHostKeysGeneratedOnce(t *testing.T) {
	utils.LinuxOnly(t)
	sshDir := "/etc/ssh/"
	sshfiles, err := os.ReadDir(sshDir)
	if err != nil {
		t.Fatalf("Couldn't read files from ssh dir")
	}

	var hashes []sshKeyHash
	for _, file := range sshfiles {
		if !strings.HasSuffix(file.Name(), "_key.pub") {
			continue
		}
		hash, err := md5Sum(sshDir + file.Name())
		if err != nil {
			t.Fatalf("Couldn't hash file: %v", err)
		}
		info, err := file.Info()
		if err != nil {
			t.Fatalf("Couldn't get file info for file %q: %v", file.Name(), err)
		}
		hashes = append(hashes, sshKeyHash{info, hash})
	}

	if err := utils.RestartAgent(utils.Context(t)); err != nil {
		t.Fatal(err)
	}

	sshfiles, err = os.ReadDir(sshDir)
	if err != nil {
		t.Fatalf("Couldn't read files from ssh dir")
	}

	var hashesAfter []sshKeyHash
	for _, file := range sshfiles {
		if !strings.HasSuffix(file.Name(), "_key.pub") {
			continue
		}
		hash, err := md5Sum(sshDir + file.Name())
		if err != nil {
			t.Fatalf("Couldn't hash file: %v", err)
		}
		info, err := file.Info()
		if err != nil {
			t.Fatalf("Couldn't get file info for file %q: %v", file.Name(), err)
		}
		hashesAfter = append(hashesAfter, sshKeyHash{info, hash})
	}

	if len(hashes) != len(hashesAfter) {
		t.Fatalf("Hashes changed after restarting guest agent")
	}

	for i := 0; i < len(hashes); i++ {
		if hashes[i].file.Name() != hashesAfter[i].file.Name() || hashes[i].hash != hashesAfter[i].hash {
			t.Fatalf("Hashes changed after restarting guest agent")
		}
	}
}

func TestHostsFile(t *testing.T) {
	utils.LinuxOnly(t)
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("couldn't get image from metadata")
	}
	if strings.Contains(image, "sles") {
		// guest-configs does not support wicked
		t.Skip("Not supported on SLES")
	}
	if strings.Contains(image, "suse") {
		// guest-configs does not support wicked
		t.Skip("Not supported on SUSE")
	}
	if strings.Contains(image, "cos") {
		// Does not have updated guest-configs with systemd-network support.
		t.Skip("Not supported on cos")
	}
	if strings.Contains(image, "ubuntu") {
		// Does not have updated guest-configs with systemd-network support.
		t.Skip("Not supported on ubuntu")
	}

	hostname, err := utils.GetMetadata(ctx, "instance", "hostname")
	if err != nil {
		t.Fatalf("Couldn't get hostname from metadata: %v", err)
	}

	testHostsEntry(t, hostname)
}

func testHostsEntry(t *testing.T, hostname string) {
	t.Helper()
	hostsFile, err := os.Open("/etc/hosts")
	if err != nil {
		t.Fatalf("os.Open(/etc/hosts) = %v, want nil", err)
	}
	defer hostsFile.Close()

	targetLineHost := fmt.Sprintf("%s %s  %s", hostname, strings.Split(hostname, ".")[0], gcomment)
	targetLineMetadata := fmt.Sprintf("%s %s  %s", "169.254.169.254", "metadata.google.internal", gcomment)

	scanner := bufio.NewScanner(hostsFile)
	var gotLines []string
	var foundHost bool
	var foundMetadata bool

	for scanner.Scan() {
		line := scanner.Text()
		gotLines = append(gotLines, line)
		if line == targetLineMetadata {
			foundMetadata = true
		} else if strings.Contains(line, targetLineHost) {
			ip := strings.TrimSpace(strings.Replace(line, targetLineHost, "", 1))
			wantLine := fmt.Sprintf("%s %s", ip, targetLineHost)
			// Check that the IP is a valid Ipv4/Ipv6 address and that the line is
			// formatted correctly.
			if net.ParseIP(ip) != nil && line == wantLine {
				foundHost = true
			}
		}

		if err := scanner.Err(); err != nil {
			t.Fatalf("scanner.Err() on /etc/hosts = %v, want nil", err)
		}
	}

	if !foundHost {
		t.Fatalf("os.ReadFile(/etc/hosts) =\n %s \nwant target host line with: <IP> %s", strings.Join(gotLines[:], "\n"), targetLineHost)
	}

	if !foundMetadata {
		t.Fatalf("os.ReadFile(/etc/hosts) =\n %s \nwant target metadata line: %q", strings.Join(gotLines[:], "\n"), targetLineMetadata)
	}
}
