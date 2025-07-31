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

	"gopkg.in/yaml.v3"
)

const (
	netplanPath = "/run/netplan/20-google-guest-agent-ethernet.yaml"
)

// netplanDropin is the netplan's drop-in configuration.
type netplanDropin struct {
	Network netplanNetwork `yaml:"network"`
}

// netplanNetwork is the netplan's drop-in network section.
type netplanNetwork struct {
	// Version is the netplan's drop-in format version.
	Version int `yaml:"version"`
	// Ethernets are the ethernet configuration entries map.
	Ethernets map[string]netplanEthernet `yaml:"ethernets,omitempty"`
}

// netplanEthernet describes the actual ethernet configuration. Refer
// https://netplan.readthedocs.io/en/stable/netplan-yaml/#properties-for-device-type-ethernets
// for more details.
type netplanEthernet struct {
	// Match is the interface's matching rule.
	Match netplanMatch `yaml:"match"`
	// DHCPv4 determines if DHCPv4 support must be enabled to such an interface.
	DHCPv4         *bool                 `yaml:"dhcp4,omitempty"`
	DHCP4Overrides *netplanDHCPOverrides `yaml:"dhcp4-overrides,omitempty"`
	// DHCPv6 determines if DHCPv6 support must be enabled to such an interface.
	DHCPv6         *bool                 `yaml:"dhcp6,omitempty"`
	DHCP6Overrides *netplanDHCPOverrides `yaml:"dhcp6-overrides,omitempty"`
}

// netplanMatch is the interface's matching rule.
type netplanMatch struct {
	// Name is the interface's name.
	Name string `yaml:"name"`
}

// netplanDHCPOverrides sets the netplan dhcp-overrides configuration.
type netplanDHCPOverrides struct {
	// When true, the domain name received from the DHCP server will be used as DNS
	// search domain over this link.
	UseDomains *bool `yaml:"use-domains,omitempty"`
}

// Verify netplan configuration.
//
// This verifies that DHCPv4 and DHCPv6 are enabled for the NIC, and that
// UseDomains is set to true only for the primary NIC.
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

	// Unmarshal the file contents.
	config := &netplanDropin{}
	if err := yaml.Unmarshal(contentBytes, config); err != nil {
		t.Fatalf("couldn't parse file %q: %v\nFile Contents: %s", netplanPath, err, content)
	}

	// Verify file contents.
	for key := range config.Network.Ethernets {
		// Skip if the key is not for the current NIC.
		if !strings.Contains(key, nic.Name) {
			continue
		}
		// We found a key that shouldn't exist.
		if !exist {
			t.Fatalf("Netplan configuration contains NIC %q, but shouldn't", nic.Name)
		}

		ethernetConfig := config.Network.Ethernets[key]
		if nic.StackType&Ipv4 != 0 {
			if ethernetConfig.DHCPv4 == nil {
				t.Fatalf("Netplan configuration %s has unexpected DHCPv4 %v, expected non-nil", netplanPath, ethernetConfig.DHCPv4)
			} else if !*ethernetConfig.DHCPv4 {
				t.Fatalf("Netplan configuration %s has unexpected DHCPv4 %v, expected true", netplanPath, ethernetConfig.DHCPv4)
			}

			if ethernetConfig.DHCP4Overrides == nil {
				t.Fatalf("Netplan configuration %s has unexpected DHCP4Overrides %v, expected non-nil", netplanPath, ethernetConfig.DHCP4Overrides)
			}
			if ethernetConfig.DHCP4Overrides.UseDomains == nil {
				t.Fatalf("Netplan configuration %s has unexpected DHCP4Overrides.UseDomains %v, expected non-nil", netplanPath, ethernetConfig.DHCP4Overrides.UseDomains)
			} else if *ethernetConfig.DHCP4Overrides.UseDomains != (nic.Index == 0) {
				t.Fatalf("Netplan configuration %s has unexpected DHCP4Overrides.UseDomains %v, expected %t", netplanPath, ethernetConfig.DHCP4Overrides.UseDomains, nic.Index == 0)
			}
		}
		if nic.StackType&Ipv6 != 0 {
			if ethernetConfig.DHCPv6 == nil {
				t.Fatalf("Netplan configuration %s has unexpected DHCPv6 %v, expected non-nil", netplanPath, ethernetConfig.DHCPv6)
			} else if !*ethernetConfig.DHCPv6 {
				t.Fatalf("Netplan configuration %s has unexpected DHCPv6 %v, expected true", netplanPath, ethernetConfig.DHCPv6)
			}

			if ethernetConfig.DHCP6Overrides == nil {
				t.Fatalf("Netplan configuration %s has unexpected DHCP6Overrides %v, expected non-nil", netplanPath, ethernetConfig.DHCP6Overrides)
			}
			if ethernetConfig.DHCP6Overrides.UseDomains == nil {
				t.Fatalf("Netplan configuration %s has unexpected DHCP6Overrides.UseDomains %v, expected non-nil", netplanPath, ethernetConfig.DHCP6Overrides.UseDomains)
			} else if *ethernetConfig.DHCP6Overrides.UseDomains != (nic.Index == 0) {
				t.Fatalf("Netplan configuration %s has unexpected DHCP6Overrides.UseDomains %v, expected %t", netplanPath, ethernetConfig.DHCP6Overrides.UseDomains, nic.Index == 0)
			}
		}
		return
	}
	// We didn't find a key for the current NIC when it should exist.
	if exist {
		t.Fatalf("Netplan configuration does not contain NIC %q", nic.Name)
	}
}
