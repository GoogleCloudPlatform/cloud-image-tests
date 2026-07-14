// Copyright 2026 Google LLC
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
	"errors"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// mwlidTestPrep modifies instance_configs.cfg to enable MWLID credential refresher and restarts agent.
func mwlidTestPrep(t *testing.T) {
	t.Helper()

	// Read existing config
	configPath := "/etc/default/instance_configs.cfg"
	if utils.IsWindows() {
		configPath = "C:\\Program Files\\Google\\Compute Engine\\instance_configs.cfg"
	}
	agentcfg, err := os.ReadFile(configPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Failed to read guest agent config: %v", err)
	}
	t.Logf("Existing guest agent config:\n%s", string(agentcfg))

	// Append [MWLID] parameters
	mwlidConfig := "\n\n[MWLID]\nenabled = true\ncredential_refresh_minutes = 1"
	agentcfg = append(agentcfg, []byte(mwlidConfig)...)

	err = os.WriteFile(configPath, agentcfg, 0640)
	if err != nil {
		t.Fatalf("Failed to write updated guest agent config: %v", err)
	}
	out, err := os.ReadFile(configPath)
	if err != nil {
		t.Errorf("Failed to read updated guest agent config: %v", err)
	}
	t.Logf("Updated guest agent config:\n%s", out)

	// Restart agent
	if err := utils.RestartAgent(utils.Context(t)); err != nil {
		t.Fatalf("Failed to restart guest agent: %v", err)
	}
	t.Logf("Guest agent restarted successfully")
	t.Logf("Waiting 30 seconds for credentials refresher job to complete")
	// Wait for credentials refresher job to complete
	time.Sleep(30 * time.Second)
	t.Logf("Slept for 30 seconds")
}

func TestMWLIDCredentials(t *testing.T) {
	// Setup the VM agent config
	mwlidTestPrep(t)

	filesToCheck := []string{"certificates.pem", "private_key.pem", "trust_bundles.json"}
	certPath := "/run/secrets/workload-spiffe-credentials/"
	if utils.IsWindows() {
		certPath = "C:\\ProgramData\\Google\\ComputeEngine\\secrets\\workload-spiffe-credentials\\"
	}

	out, err := os.ReadDir(certPath)
	if err != nil {
		t.Errorf("Failed to read directory %q: %v", certPath, err)
	}
	t.Logf("Contents of %q:\n%s", certPath, out)

	// Add checks for private_key.pem, certificates.pem, etc.
	for _, f := range filesToCheck {
		filePath := certPath + f

		// os.Stat returns info about the file, or an error if it doesn't exist
		if _, err := os.Stat(filePath); err != nil {
			if os.IsNotExist(err) {
				t.Errorf("Certificate file %s not found.", filePath)
			} else {
				// Catches permission issues or other OS errors
				t.Errorf("Error checking file %s: %v", filePath, err)
			}
			continue
		}

		t.Logf("Certificate file %s successfully found", filePath)
	}
}
