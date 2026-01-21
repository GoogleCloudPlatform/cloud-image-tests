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
	utils.WindowsOnly(t)
	services := []string{"GCEAgentManager"}
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Failed to get image: %v", err)
	}
	if strings.Contains(image, "guest-agent-stable") {
		// Service is enabled only on stable images.
		services = append(services, "GCEAgent")
	} else {
		// Service is disabled on stable images.
		services = append(services, "GCEWindowsCompatManager")
	}

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
