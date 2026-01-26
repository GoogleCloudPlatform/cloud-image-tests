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
	"encoding/json"
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
	if !strings.Contains(image, "guest-agent") && (utils.IsCOS(image) || utils.IsUbuntu(image) || utils.IsSLES(image)) {
		// Old agent package installed on COS, SLES and Ubuntu images only has google-guest-agent service.
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
	services := []string{"GCEAgentManager"}
	// TODO(b/478951370): Remove this exception once the bug is fixed.
	if strings.Contains(image, "guest-agent-stable") || strings.Contains(image, "gce-staging-images") {
		// Service is enabled only on stable images. Some staging images still have
		// the old agent.
		services = append(services, "GCEAgent")
	} else {
		// Service is disabled on stable images.
		services = append(services, "GCEWindowsCompatManager")
	}

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
