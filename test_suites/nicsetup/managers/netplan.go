// Copyright 2025 Google LLC.
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

package managers

import (
	"os"
	"strings"
	"testing"
)

const (
	netplanPath = "/run/netplan/20-google-guest-agent-ethernet.yaml"
)

// TODO(b/431297188): Verify netplan configuration.
func testNetplan(t *testing.T, nic EthernetInterface, exist bool) {
	t.Helper()

	// Netplan file has all the configurations for NICs. So we read the contents
	// of the file instead.
	contentBytes, err := os.ReadFile(netplanPath)
	if err != nil {
		if !exist {
			return
		}
		t.Fatalf("couldn't read file %q: %v", netplanPath, err)
	}
	content := string(contentBytes)

	if strings.Contains(content, nic.Name) != exist {
		t.Fatalf("Netplan configuration %s has unexpected NIC %s:\n %s", netplanPath, nic.Name, content)
	}
}
