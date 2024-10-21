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
	"runtime"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
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
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("couldn't determine local hostname")
	}

	if hostname != shortname {
		return fmt.Errorf("hostname does not match metadata. Expected: %q got: %q", shortname, hostname)
	}

	// If hostname is FQDN then lots of tools (e.g. ssh-keygen) have issues
	if strings.Contains(hostname, ".") {
		return fmt.Errorf("hostname contains '.'")
	}
	return nil
}

func TestHostname(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	metadataHostname, err := utils.GetMetadata(ctx, "instance", "hostname")
	if err != nil {
		t.Fatalf(" still couldn't determine metadata hostname")
	}

	// 'hostname' in metadata is fully qualified domain name.
	shortname := strings.Split(metadataHostname, ".")[0]

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
	ctx, cancel := utils.Context(t)
	defer cancel()
	image, err := utils.GetMetadata(ctx, "instance", "image")
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
	ctx, cancel := utils.Context(t)
	defer cancel()

	metadataHostname, err := utils.GetMetadata(ctx, "instance", "hostname")
	if err != nil {
		t.Fatalf("couldn't determine metadata hostname")
	}

	// Get the hostname with FQDN.
	cmd := exec.Command("/bin/hostname", "-f")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("hostname command failed")
	}
	hostname := strings.TrimRight(string(out), " \n")

	if hostname != metadataHostname {
		t.Errorf("hostname does not match metadata. Expected: %q got: %q", metadataHostname, hostname)
	}
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

	ctx, cancel := utils.Context(t)
	defer cancel()
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata")
	}

	var restart string
	switch {
	case strings.Contains(image, "rhel-6"), strings.Contains(image, "centos-6"):
		restart = "initctl"
	default:
		restart = "systemctl"
	}

	cmd := exec.Command(restart, "restart", "google-guest-agent")
	err = cmd.Run()
	if err != nil {
		t.Errorf("Failed to restart guest agent: %v", err)
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
	ctx, cancel := utils.Context(t)
	defer cancel()
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
