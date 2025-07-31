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
	"fmt"
	"os"
	"testing"

	"github.com/go-ini/ini"
)

const (
	systemdNetworkdPath = "/usr/lib/systemd/network/20-%s-google-guest-agent.network"
)

// networkdConfig wraps the interface configuration for systemd-networkd.
// Ultimately the structure will be unmarshalled into a .ini file.
type networkdConfig struct {
	// Match is the systemd-networkd ini file's [Match] section.
	Match networkdMatchConfig
	// Network is the systemd-networkd ini file's [Network] section.
	Network networkdNetworkConfig
	// DHCPv4 is the systemd-networkd ini file's [DHCPv4] section.
	DHCPv4 *networkdDHCPConfig `ini:",omitempty"`
}

// networkdMatchConfig is the systemd-networkd ini file's [Match] section.
type networkdMatchConfig struct {
	// Name is the name of the interface to match.
	Name string
}

// networkdNetworkConfig is the systemd-networkd ini file's [Network] section.
type networkdNetworkConfig struct {
	// DHCP determines the ipv4/ipv6 protocol version for use with dhcp.
	DHCP string `ini:"DHCP,omitempty"`
	// DNSDefaultRoute is used to determine if the link's configured DNS servers
	// are used for resolving domain names that do not match any link's domain.
	DNSDefaultRoute bool
}

// networkdDHCPConfig is the systemd-networkd ini file's [DHCPvX] section.
type networkdDHCPConfig struct {
	// RoutesToDNS defines if routes to the DNS servers received from the DHCP
	// should be configured/installed.
	RoutesToDNS bool
	// RoutesToNTP defines if routes to the NTP servers received from the DHCP
	// should be configured/installed.
	RoutesToNTP bool
}

// Verify systemd-networkd configuration.
func testSystemdNetworkd(t *testing.T, nic EthernetInterface, exist bool) {
	t.Helper()

	file := fmt.Sprintf(systemdNetworkdPath, nic.Name)
	verifyFileExists(t, file, exist)

	if exist {
		// Read and map the file.
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("couldn't read file %q: %v", file, err)
		}
		content := string(contentBytes)
		config := &networkdConfig{}
		if err := ini.MapTo(config, contentBytes); err != nil {
			t.Fatalf("couldn't parse file %q: %v\nFile Contents: %s", file, err, content)
		}

		// Verify file contents.
		if config.Match.Name != nic.Name {
			t.Fatalf("systemd-networkd configuration %s has unexpected NIC %q, expected %q", file, config.Match.Name, nic.Name)
		}
		expectedDHCP := "ipv4"
		if nic.StackType&Ipv6 != 0 {
			expectedDHCP = "yes"
		}
		if config.Network.DHCP != expectedDHCP {
			t.Fatalf("systemd-networkd configuration %s has unexpected DHCP protocol %q, expected %q", file, config.Network.DHCP, expectedDHCP)
		}
		if config.Network.DNSDefaultRoute != (nic.Index == 0) {
			t.Fatalf("systemd-networkd configuration %s has unexpected DNSDefaultRoute %t, expected %t", file, config.Network.DNSDefaultRoute, nic.Index == 0)
		}
	}
}
