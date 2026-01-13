// Copyright 2023 Google LLC
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

package oslogin

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func TestOsLoginEnabled(t *testing.T) {
	if err := isOsLoginEnabled(utils.Context(t)); err != nil {
		t.Fatal(err.Error())
	}
}

func TestOsLoginDisabled(t *testing.T) {
	// Check OS Login not enabled in /etc/nsswitch.conf
	data, err := os.ReadFile("/etc/nsswitch.conf")
	if err != nil {
		t.Fatalf("cannot read /etc/nsswitch.conf")
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "passwd:") && strings.Contains(line, "oslogin") {
			t.Errorf("OS Login NSS module wrongly included in /etc/nsswitch.conf when disabled.")
		}
	}

	// Check AuthorizedKeys Command
	data, err = os.ReadFile("/etc/ssh/sshd_config")
	if err != nil {
		t.Fatalf("cannot read /etc/ssh/sshd_config")
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "AuthorizedKeysCommand") && strings.Contains(line, "/usr/bin/google_authorized_keys") {
			t.Errorf("OS Login AuthorizedKeysCommand directive wrongly exists when disabled.")
		}
	}

	if err = testSSHDPamConfig(utils.Context(t)); err != nil {
		t.Fatalf("error checking pam config: %v", err)
	}
}

func TestGetentPasswdOsloginUser(t *testing.T) {
	testUsername, _, testUserEntry, err := getTestUserEntry(utils.Context(t))
	if err != nil {
		t.Fatalf("failed to get test user entry: %v", err)
	}

	cmd := exec.Command("getent", "passwd", testUsername)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("getent command failed %v", err)
	}
	if !strings.Contains(string(out), testUserEntry) {
		t.Errorf("getent passwd output does not contain %s", testUserEntry)
	}
}

func rootUserEntry(ctx context.Context, t *testing.T) string {
	t.Helper()
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("failed to get image: %v", err)
	}
	// https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/10/html-single/10.0_release_notes/index#:~:text=GECOS%20field%20for%20root%20user%20is%20changed%20to%20Super%20User
	el10basedOSs := []string{"rhel-10", "centos-stream-10", "rocky-linux-10", "oracle-linux-10", "almalinux-10"}
	for _, os := range el10basedOSs {
		if strings.Contains(image, os) {
			t.Logf("EL-10 based image detected %q, using Super User as GECOS for root user", image)
			return "root:x:0:0:Super User:/root:"
		}
	}
	t.Logf("EL-10 based image not detected %q, using root as GECOS for root user", image)
	return "root:x:0:0:root:/root:"
}

// PreTestCacheCheck checks if the OS Login cache is empty and refreshes it if necessary.
// TODO: b/474640709 - remove this function once the bug is fixed.
func PreTestCacheCheck(t *testing.T) {
	t.Helper()
	cacheFile := "/etc/oslogin_passwd.cache"
	const maxRetries = 3

	for i := 0; i < maxRetries; i++ {
		fileInfo, err := os.Stat(cacheFile)

		if err != nil {
			if os.IsNotExist(err) {
				t.Logf("OS Login cache file %s does not exist. Refresh attempt %d/%d.", cacheFile, i+1, maxRetries)
			} else {
				t.Logf("Error stating OS Login cache file %s: %v. Refresh attempt %d/%d.", cacheFile, err, i+1, maxRetries)
			}
		} else if fileInfo.Size() == 0 {
			t.Logf("OS Login cache file %s is empty. Refresh attempt %d/%d.", cacheFile, i+1, maxRetries)
		} else {
			t.Logf("OS Login cache file %s is not empty. Size: %d bytes.", cacheFile, fileInfo.Size())
			return // Cache is in a good state
		}
		// If we haven't returned, we need to refresh.
		t.Logf("Attempting to refresh cache, attempt %d/%d...", i+1, maxRetries)
		cmd := exec.Command("sudo", "/usr/bin/google_oslogin_nss_cache")
		out, refreshErr := cmd.CombinedOutput()
		if refreshErr != nil {
			t.Logf("Failed to run OS Login cache refresh: %v\nOutput: %s", refreshErr, string(out))
		}
		t.Logf("OS Login cache refresh command output:\n%s", string(out))
		if i < maxRetries-1 { // Don't sleep after the last attempt in the loop
			// Give a moment for the cache file to be written
			time.Sleep(2 * time.Second)
		}
	}

	// Final check after retries
	fileInfo, err := os.Stat(cacheFile)
	if err != nil || fileInfo.Size() == 0 {
		t.Logf("OS Login cache file %s is still not populated after %d refresh attempts.", cacheFile, maxRetries)
	}
}

func TestGetentPasswdAllUsers(t *testing.T) {
	ctx := utils.Context(t)

	// Check and refresh cache if empty
	PreTestCacheCheck(t)

	_, _, testUserEntry, err := getTestUserEntry(ctx)
	if err != nil {
		t.Fatalf("failed to get test user entry: %v", err)
	}

	cmd := exec.Command("getent", "passwd")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("getent command failed %v", err)
	}
	if rootEntry := rootUserEntry(ctx, t); !strings.Contains(string(out), rootEntry) {
		t.Errorf("getent passwd output does not contain user root; got %q, want containing %q", string(out), rootEntry)
	}
	if !strings.Contains(string(out), "nobody:x:") {
		t.Errorf("getent passwd output does not contain user nobody; got %q, want containing %q", string(out), "nobody:x:")
	}
	if !strings.Contains(string(out), testUserEntry) {
		t.Errorf("getent passwd output does not contain the test user; got %q, want %q", string(out), testUserEntry)
	} else {
		t.Logf("Test user entry found in getent passwd output: %q", testUserEntry)
	}
}

func TestGetentPasswdOsloginUID(t *testing.T) {
	_, testUUID, testUserEntry, err := getTestUserEntry(utils.Context(t))
	if err != nil {
		t.Fatalf("failed to get test user entry: %v", err)
	}

	cmd := exec.Command("getent", "passwd", testUUID)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("getent command failed %v", err)
	}
	if !strings.Contains(string(out), testUserEntry) {
		t.Errorf("getent passwd output does not contain %s", testUserEntry)
	}
}

func TestGetentPasswdLocalUser(t *testing.T) {
	cmd := exec.Command("getent", "passwd", "nobody")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("getent command failed %v", err)
	}
	if !strings.Contains(string(out), "nobody:x:") {
		t.Errorf("getent passwd output does not contain user nobody")
	}
}

func TestGetentPasswdInvalidUser(t *testing.T) {
	cmd := exec.Command("getent", "passwd", "__invalid_user__")
	err := cmd.Run()
	if err.Error() != "exit status 2" {
		t.Errorf("getent passwd did not give error on invalid user")
	}
}
