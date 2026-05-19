// Copyright 2025 Google LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     https://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package guestagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"google.golang.org/api/compute/v1"
)

type diagnosticsEntry struct {
	SignedURL string
	ExpireOn  string
	Trace     bool
}

const (
	guestAgentManagerService = "google-guest-agent-manager.service"
	guestAgentService        = "google-guest-agent.service"
	rsaKeyPath               = "/etc/ssh/ssh_host_rsa_key"
	ed25519KeyPath           = "/etc/ssh/ssh_host_ed25519_key"
	ecdsaKeyPath             = "/etc/ssh/ssh_host_ecdsa_key"
)

func TestDiagnostic(t *testing.T) {
	entry := &diagnosticsEntry{
		SignedURL: "https://teststorage.googleapis.com/test-bucket-1/test-object-1",
		ExpireOn:  time.Now().Add(10 * time.Minute).Format(time.RFC3339),
	}
	json, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("Failed to marshal diagnostics entry %+v to JSON: %v", entry, err)
	}

	name, err := utils.GetInstanceName(utils.Context(t))
	if err != nil {
		t.Fatalf("Failed to get instance name: %v", err)
	}

	m := utils.GetInstanceMetadata(t, name)
	diagEntry := string(json)
	m.Items = append(m.Items, &compute.MetadataItems{Key: "diagnostics", Value: &diagEntry})
	utils.SetInstanceMetadata(t, name, m)
	time.Sleep(time.Minute)

	var found bool
	for i := 0; i <= 5; i++ {
		matches, err := filepath.Glob(filepath.Join(`C:\Windows\TEMP`, "diagnostics*", "logs.zip"))
		if err == nil && len(matches) > 0 {
			found = true
			t.Logf("Found diagnostics entry: %s", matches[0])
			break
		}
		time.Sleep(time.Minute)
	}

	if !found {
		t.Errorf("Failed to find diagnostic logs.zip after the timeout")
	}

	checkRegCmd := `Get-ItemProperty -Path HKLM:\SOFTWARE\Google\ComputeEngine`
	processStatus, err := utils.RunPowershellCmd(checkRegCmd)
	if err != nil {
		t.Fatalf(`Failed to read HKLM:\SOFTWARE\Google\ComputeEngine registry: %v`, err)
	}
	if !strings.Contains(processStatus.Stdout, "Diagnostics") {
		t.Errorf("Failed to find diagnostic entries in registry, out: %+v", processStatus)
	}
}

func TestServiceConfig(t *testing.T) {
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Failed to get image: %v", err)
	}

	if utils.IsWindows() {
		testServiceConfigWindows(t, image)
	} else {
		testServiceConfigLinux(t, image)
	}
}

func testServiceConfigLinux(t *testing.T, image string) {
	afterDependencies := []string{"network-online.target", "NetworkManager.service", "systemd-networkd.service"}
	services := []string{"google-guest-agent-manager", "google-guest-agent", "google-guest-compat-manager"}

	// TODO(b/478951370): Remove this exception once the bug is fixed.
	isOldAgentDistro := utils.IsCOS(image) || utils.IsUbuntu(image) || utils.IsSLES(image) || utils.IsOpenSUSE(image)
	if !strings.Contains(image, "guest-agent") && isOldAgentDistro {
		// Old agent package installed on COS, SLES, OpenSUSE and Ubuntu images only has google-guest-agent service.
		services = []string{"google-guest-agent"}
	}

	t.Logf("Testing service config for image: %s, services: %v", image, services)

	for _, service := range services {
		cmd := exec.Command("systemctl", "show", service, "-p", "After", "--value", "--no-pager")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("Failed to get service %q status: %v, output: %q", service, err, string(out))
		}

		foundDependencies := strings.TrimSpace(string(out))
		var notfound []string

		for _, afterDependency := range afterDependencies {
			if !strings.Contains(foundDependencies, afterDependency) {
				notfound = append(notfound, afterDependency)
			}
		}

		if len(notfound) > 0 {
			t.Errorf("Service %q is missing dependencies: %v, output: %s", service, notfound, string(out))
		}
	}
}

func testServiceConfigWindows(t *testing.T, image string) {
	t.Helper()
	services := []string{"GCEAgentManager", "GCEWindowsCompatManager"}
	t.Logf("Testing service config for image: %s, services: %v", image, services)

	for _, service := range services {
		cmd := exec.Command("sc", "qc", service)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("Failed to get service status: %v, output: %+v", err, string(out))
		}
		lines := strings.Split(string(out), "\r\n")
		found := false
		for _, line := range lines {
			if strings.Contains(line, "AUTO_START") && strings.Contains(line, "DELAYED") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Service %s is not in AUTO_START (DELAYED) state, output: %s", service, string(out))
		}
	}
}

// TestSSHHostKeyExistence verifies that standard SSH host keys have been generated.
func TestSSHHostKeyExistence(t *testing.T) {
	expectedKeys := []string{rsaKeyPath, ed25519KeyPath}

	for _, key := range expectedKeys {
		if _, err := os.Stat(key); err != nil {
			t.Errorf("Required SSH host key %s is missing: %v", key, err)
		} else {
			t.Logf("Found SSH host key: %s", key)
		}
	}
}

// TestSSHHostKeyTimingVsAgent verifies that SSH host keys are written
// by the Google Guest Agent before it marks itself ready.
func TestSSHHostKeyTimingVsAgent(t *testing.T) {
	ctx := context.Background()

	// Get the modification time of the RSA host key
	info, err := os.Stat(rsaKeyPath)
	if err != nil {
		t.Fatalf("Failed to stat %s: %v", rsaKeyPath, err)
	}
	keyTime := info.ModTime()
	t.Logf("Host key %s modification time: %v", rsaKeyPath, keyTime)

	// Determine which service is active and get its timestamps
	activeService := guestAgentService
	agentReadyTime, err := getServiceTimestamps(ctx, activeService)
	if err != nil {
		t.Logf("Failed to get timestamps for %s: %v. Trying %s...", activeService, err, guestAgentManagerService)

		activeService = guestAgentManagerService
		agentReadyTime, err = getServiceTimestamps(ctx, activeService)
		if err != nil {
			t.Fatalf("Failed to get start times for both %s and %s: %v", guestAgentService, guestAgentManagerService, err)
		}
	}

	t.Logf("%s ActiveEnterTimestamp: %v", activeService, agentReadyTime)
	// Assertion: Key should be written BEFORE the guest agent marks itself ready.
	if keyTime.After(agentReadyTime) {
		t.Errorf("Host keys written (%v) AFTER %s became ready (%v). Keys must exist before agent is ready.", keyTime, activeService, agentReadyTime)
	} else {
		t.Logf("PASS: Host keys written (%v) before %s ActiveEnterTimestamp (%v)", keyTime, activeService, agentReadyTime)
	}
}

// getServiceTimestamps queries systemd for the ActiveEnterTimestamp of a given service.
func getServiceTimestamps(ctx context.Context, service string) (time.Time, error) {
	cmd := exec.CommandContext(ctx, "systemctl", "show", service, "-p", "ActiveEnterTimestamp", "--value")
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, fmt.Errorf("systemctl show failed for %s: %v", service, err)
	}

	timeStr := strings.TrimSpace(string(out))
	if timeStr == "" {
		return time.Time{}, fmt.Errorf("empty timestamp returned for %s", service)
	}

	// systemd ActiveEnterTimestamp format: "Mon 2023-10-23 15:34:12 UTC"
	layout := "Mon 2006-01-02 15:04:05 MST"
	parsedTime, err := time.Parse(layout, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse time %q: %v", timeStr, err)
	}

	return parsedTime, nil
}
