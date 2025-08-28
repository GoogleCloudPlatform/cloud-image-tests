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
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Verify dhclient configuration.
//
// If a DHCP client process is running for a NIC, the guest agent should not
// spin up a new process. So in this case we just verify that a process exists
// for each NIC according to their stack type.
func testDhclient(t *testing.T, nic EthernetInterface, exist bool) {
	t.Helper()

	// Process for primary NIC should always be running, except on Ubuntu 18.04,
	// where dhclient process may not necessarily be running.
	if nic.Index == 0 && !isUbuntu1804(t) {
		exist = true
	}

	// dhclient has a special case. We check if any dhclient process for the NIC
	// is running.
	// Find IPv4 dhclient process.
	processes, err := exec.Command("pgrep", "dhclient", "-a").Output()
	if err != nil {
		if !exist {
			return
		}
		t.Fatalf("Failed to get dhclient processes: %v", err)
	}

	// We check if core plugin is enabled. On versions of the guest agent that
	// don't have core plugin enabled, there is a bug where multiple dhclient
	// processes can be running for the same NIC.
	coreDisabled := utils.IsCoreDisabled()
	if coreDisabled {
		t.Logf("Core plugin disabled, skipping dhclient duplicate process check.")
	}

	var ipv4Process, ipv6Process bool
	for _, process := range strings.Split(string(processes), "\n") {
		fields := strings.Fields(process)
		if len(fields) < 2 {
			continue
		}
		if !strings.Contains(fields[1], "dhclient") {
			continue
		}

		t.Logf("Process: %s", process)
		// Skip processes that are not for the current NIC.
		if forCurrNic := slices.Contains(fields, nic.Name); !forCurrNic {
			continue
		}

		// Check if the process is IPv4 or IPv6.
		if slices.Contains(fields, "-6") {
			if ipv6Process && !coreDisabled {
				t.Errorf("Found multiple IPv6 dhclient processes for NIC %q", nic.Name)
			}
			ipv6Process = true
		} else {
			if ipv4Process && !coreDisabled {
				t.Errorf("Found multiple IPv4 dhclient processes for NIC %q", nic.Name)
			}
			ipv4Process = true
		}
	}

	// Older ubuntu versions only has dhclient running for IPv4 on primary NIC
	// by default. So we assume the IPv6 process exists to let the test pass.
	if strings.Contains(image, "ubuntu") && nic.Index == 0 && nic.StackType != Ipv4 {
		ipv6Process = true
	}

	switch nic.StackType {
	case Ipv4:
		if exist != ipv4Process {
			t.Errorf("NIC %q: Found IPv4 dhclient process: %t, expected: %t", nic.Name, ipv4Process, exist)
		}
	case Ipv4Ipv6:
		if exist != ipv4Process {
			t.Errorf("NIC %q: Found IPv4 dhclient process: %t, expected: %t", nic.Name, ipv4Process, exist)
		}
		if exist != ipv6Process {
			t.Errorf("NIC %q: Found IPv6 dhclient process: %t, expected: %t", nic.Name, ipv6Process, exist)
		}
	case Ipv6:
		if exist != ipv6Process {
			t.Errorf("NIC %q: Found IPv6 dhclient process: %t, expected: %t", nic.Name, ipv6Process, exist)
		}
	}
}
