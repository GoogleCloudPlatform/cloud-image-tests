// Copyright 2026 Google LLC.
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

//go:build linux

package nicsetup

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestMetadataHostsCompliance validates that /etc/hosts is correctly configured
// based on the stack types detected on the primary interface (nic0).
func TestMetadataHostsCompliance(t *testing.T) {
	t.Logf("%s: Verifying /etc/hosts metadata compliance", getCurrentTime())

	hasGlobalV4, hasGlobalV6 := getStackType(t)
	t.Logf("Detected stack type: Global IPv4: %v, Global IPv6: %v", hasGlobalV4, hasGlobalV6)

	hostsBytes, err := os.ReadFile("/etc/hosts")
	if err != nil {
		t.Fatalf("failed to read /etc/hosts: %v", err)
	}
	hostsContent := string(hostsBytes)
	t.Logf("/etc/hosts content:\n%s", hostsContent)

	hasV4MDS := strings.Contains(hostsContent, "169.254.169.254 metadata.google.internal")
	hasV6MDS := strings.Contains(hostsContent, "fd20:ce::254 metadata.google.internal")

	dualStack := hasGlobalV4 && hasGlobalV6
	ipv4Only := hasGlobalV4 && !hasGlobalV6
	ipv6Only := !hasGlobalV4 && hasGlobalV6
	stackName := "Unknown"

	if dualStack {
		stackName = "DualStack"
		if !hasV4MDS {
			t.Errorf("%s: MUST contain 169.254.169.254 metadata.google.internal", stackName)
		}
		if hasV6MDS {
			t.Errorf("%s: MUST NOT contain fd20:ce::254 metadata.google.internal (IPv6 MDS disabled on dual-stack)", stackName)
		}
	} else if ipv6Only {
		stackName = "IPv6Only"
		if !hasV6MDS {
			t.Errorf("%s: MUST contain fd20:ce::254 metadata.google.internal", stackName)
		}
	} else if ipv4Only {
		stackName = "IPv4Only"
		if !hasV4MDS {
			t.Errorf("%s: MUST contain 169.254.169.254 metadata.google.internal", stackName)
		}
		if hasV6MDS {
			t.Errorf("%s: MUST NOT contain fd20:ce::254 metadata.google.internal", stackName)
		}
	} else {
		stackName = "NoGlobalIP"
		if hasV4MDS || hasV6MDS {
			t.Errorf("%s: unexpected MDS entries found", stackName)
		}
	}
	t.Logf("Detected stack type: %s", stackName)
}

// TestMetadataResolutionAndReachability verifies MDS reachability & content specifically on nic0.
func TestMetadataResolutionAndReachability(t *testing.T) {
	t.Logf("%s: Verifying MDS reachability & content via hostname", getCurrentTime())

	hasGlobalV4, hasGlobalV6 := getStackType(t)
	if !hasGlobalV4 && !hasGlobalV6 {
		t.Skip("Skipping reachability tests: No global IP addresses detected on primary interface.")
		return
	}

	curlCmd := exec.Command("curl", "-sSf", "-m", "5", "-H", "Metadata-Flavor: Google", "http://metadata.google.internal/computeMetadata/v1/instance/hostname")
	output, err := curlCmd.CombinedOutput()

	if err != nil {
		t.Fatalf("MDS reachability via HOSTNAME failed (Stack: v4:%v, v6:%v): %v\nOutput: %s", hasGlobalV4, hasGlobalV6, err, string(output))
	} else {
		t.Logf("MDS hostname resolution and reachability successful: %s, detected stack: v4:%v, v6:%v", strings.TrimSpace(string(output)), hasGlobalV4, hasGlobalV6)
	}
}

// TestMDSHostsCorrection executes a negative test on Dual-Stack VMs by manually
// injecting a forbidden IPv6 MDS entry, triggering the network dispatcher, and
// verifying that the core worker successfully scrubs the file.
func TestMDSHostsCorrection(t *testing.T) {
	t.Logf("%s: Verifying /etc/hosts correction on dual stack (Negative Test)", getCurrentTime())

	hasGlobalV4, hasGlobalV6 := getStackType(t)
	if !(hasGlobalV4 && hasGlobalV6) {
		t.Skip("Skipping negative test: Not a dual-stack VM.")
		return
	}

	badEntry := "fd20:ce::254 metadata.google.internal"
	hostsFile := "/etc/hosts"

	// Append the forbidden IPv6 entry to /etc/hosts
	hostsBytes, _ := os.ReadFile(hostsFile)
	originalContent := string(hostsBytes)
	if !strings.Contains(originalContent, badEntry) {
		f, err := os.OpenFile(hostsFile, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatalf("Failed to open /etc/hosts for append: %v", err)
		}
		if _, err := f.WriteString("\n" + badEntry + "\n"); err != nil {
			t.Fatalf("Failed to write bad entry to /etc/hosts: %v", err)
		}
		f.Close()
		t.Logf("Appended forbidden IPv6 entry to /etc/hosts")
	}

	// Restart the network service (Triggers dispatcher hooks)
	refreshNetworkState(t)

	// Wait for the asynchronous script execution to acquire lock and complete
	time.Sleep(5 * time.Second)

	// Verify the forbidden line was removed
	hostsBytes, err := os.ReadFile(hostsFile)
	if err != nil {
		t.Fatalf("failed to read /etc/hosts: %v", err)
	}
	currentContent := string(hostsBytes)

	if strings.Contains(currentContent, badEntry) {
		t.Errorf("Negative Test Failed: Dispatcher script failed to remove the forbidden IPv6 MDS entry.")
	} else {
		t.Logf("Negative Test Passed: Forbidden entry successfully scrubbed.")
	}

	// Ensure the required IPv4 entry survived
	if !strings.Contains(currentContent, "169.254.169.254 metadata.google.internal") {
		t.Errorf("Negative Test Failed: Dispatcher script accidentally removed the required IPv4 MDS entry during cleanup.")
	}
}

// refreshNetworkState detects the active network service and forces a
// re-run of dispatcher scripts to verify configuration enforcement.
func refreshNetworkState(t *testing.T) {
	t.Helper()

	// Detect primary interface dynamically
	outIface, _ := exec.Command("bash", "-c", "ip route show default | head -n 1 | awk '{print $5}'").Output()
	primaryIface := strings.TrimSpace(string(outIface))
	if primaryIface == "" {
		outIface6, _ := exec.Command("bash", "-c", "ip -6 route show default | head -n 1 | awk '{print $5}'").Output()
		primaryIface = strings.TrimSpace(string(outIface6))
	}
	if primaryIface == "" {
		t.Errorf("Failed to detect primary interface")
		return
	}

	// Check for systemd-networkd (Primary for Debian 12+/Ubuntu)
	_, errNetworkd := exec.LookPath("networkd-dispatcher")
	isNetworkdActive := exec.Command("systemctl", "is-active", "--quiet", "systemd-networkd").Run() == nil

	if errNetworkd == nil && isNetworkdActive {
		t.Log("Detected systemd-networkd; triggering via networkd-dispatcher")
		cmd := exec.Command("sudo", "networkd-dispatcher", "--run-startup-triggers")
		if err := cmd.Start(); err != nil {
			t.Errorf("networkd-dispatcher trigger failed to start: %v", err)
		}
		return
	}

	// Check for NetworkManager (Primary for RHEL/CentOS/Fedora)
	_, errNM := exec.LookPath("nmcli")
	isNMActive := exec.Command("systemctl", "is-active", "--quiet", "NetworkManager").Run() == nil

	if errNM == nil && isNMActive {
		t.Logf("Detected NetworkManager; simulating dispatcher event for %s", primaryIface)

		hookPath := "/etc/NetworkManager/dispatcher.d/google_hostname.sh"

		// Verify the hook exists before calling it
		if _, err := os.Stat(hookPath); err == nil {
			// Call the hook exactly as NM does: script <interface> <action>
			cmd := exec.Command("sudo", hookPath, primaryIface, "up")
			if err := cmd.Start(); err != nil {
				t.Errorf("Simulated NetworkManager hook failed to start: %v", err)
			}
			return
		}
	}

	// Check for dhclient
	isDhclientActive := exec.Command("pgrep", "dhclient").Run() == nil

	if isDhclientActive {
		t.Logf("Detected active dhclient; forcing release and restart on %s", primaryIface)

		// Release the lease properly
		_ = exec.Command("sudo", "dhclient", "-r", primaryIface).Run()

		// Kill any rogue/zombie dhclient processes left behind
		// This is needed to ensure that the next dhclient process is the only one running.
		_ = exec.Command("sudo", "pkill", "-9", "dhclient").Run()

		// Bring the interface back up cleanly using the system's native network manager
		// rather than launching a raw dhclient process.
		restartCmd := exec.Command("sudo", "ifup", "--force", primaryIface)
		if err := restartCmd.Start(); err != nil {
			t.Logf("Warning: ifup failed, falling back to raw dhclient start: %v", err)
			_ = exec.Command("sudo", "dhclient", primaryIface).Start()
		}
		return
	}
	// Check for wicked (Primary for SUSE/SLES)
	_, errWicked := exec.LookPath("wicked")
	isWickedActive := exec.Command("systemctl", "is-active", "--quiet", "wicked").Run() == nil

	if errWicked == nil && isWickedActive {
		t.Logf("Detected wicked; simulating post-up hook for %s", primaryIface)

		hookPath := "/etc/sysconfig/network/scripts/google_up.sh"

		// 1. Check for errors/missing files first
		if _, err := os.Stat(hookPath); err != nil {
			t.Errorf("wicked is active, but hook script is missing at %s", hookPath)
			return
		}

		// 2. Proceed with "Happy path" (unindented)
		cmd := exec.Command("sudo", hookPath, primaryIface)
		if err := cmd.Start(); err != nil {
			t.Errorf("Simulated wicked hook failed to start: %v", err)
		}
		return
	}

	// Update the fatal string to include dhclient
	t.Fatalf("No active network manager (systemd-networkd, NetworkManager, or dhclient) found to trigger dispatcher scripts.")
}

// getStackType determines the IP stack capabilities of the PRIMARY interface only,
// as Google Cloud Metadata Server (MDS) routing is strictly bound to nic0.
func getStackType(t *testing.T) (v4 bool, v6 bool) {
	t.Helper()
	t.Logf("%s: Determining stack type for primary interface", getCurrentTime())

	// Identify the primary interface (checking both IPv4 and IPv6 routes)
	outIface, _ := exec.Command("bash", "-c", "ip route show default | head -n 1 | awk '{print $5}'").Output()
	primaryIface := strings.TrimSpace(string(outIface))
	if primaryIface == "" {
		outIface6, _ := exec.Command("bash", "-c", "ip -6 route show default | head -n 1 | awk '{print $5}'").Output()
		primaryIface = strings.TrimSpace(string(outIface6))
	}
	if primaryIface == "" {
		primaryIface = "eth0" // Fallback
	}

	t.Logf("Evaluating stack type specifically for primary interface: %s", primaryIface)

	// Check for global IPv4 exclusively on the primary interface
	cmdV4 := fmt.Sprintf("ip -4 addr show dev %s scope global | grep -v '169.254' | grep -q 'inet '", primaryIface)
	if err := exec.Command("bash", "-c", cmdV4).Run(); err == nil {
		v4 = true
	}

	// Check for global IPv6 exclusively on the primary interface
	cmdV6 := fmt.Sprintf("ip -6 addr show dev %s scope global | grep -q 'inet6'", primaryIface)
	if err := exec.Command("bash", "-c", cmdV6).Run(); err == nil {
		v6 = true
	}

	return v4, v6
}
