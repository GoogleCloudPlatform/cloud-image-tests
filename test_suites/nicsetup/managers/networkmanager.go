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
	networkManagerFile = "/etc/NetworkManager/system-connections/google-guest-agent-%s.nmconnection"
)

// nmConfig represents the NetworkManager configuration.
type nmConfig struct {
	Connection nmConnection `ini:"connection"`
	IPv4       nmIPv4Config `ini:"ipv4"`
	IPv6       nmIPv6Config `ini:"ipv6"`
}

// nmConnection represents the NetworkManager configuration's connection section.
type nmConnection struct {
	// InterfaceName is the name of the interface to configure.
	InterfaceName string `ini:"interface-name"`
	// ID is the unique ID for this connection.
	ID string `ini:"id"`
	// ConnType is the type of connection (i.e. ethernet).
	ConnType string `ini:"type"`
}

// nmIPv4Config represents the NetworkManager configuration's IPv4 section.
type nmIPv4Config struct {
	// Method is the IPv4 method to configure.
	Method string `ini:"method"`
}

// nmIPv6Config represents the NetworkManager configuration's IPv6 section.
type nmIPv6Config struct {
	// Method is the IPv6 method to configure.
	Method string `ini:"method"`
}

// Verify NetworkManager configuration.
//
// The guest agent writes a file for each NIC. These files should match the NIC
// for which they're written.
func testNetworkManager(t *testing.T, nic EthernetInterface, exist bool) {
	t.Helper()

	file := fmt.Sprintf(networkManagerFile, nic.Name)
	verifyFileExists(t, file, exist)

	if exist {
		// Read and map the file.
		contentBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("couldn't read file %q: %v", file, err)
		}
		content := string(contentBytes)
		config := &nmConfig{}
		if err := ini.MapTo(config, contentBytes); err != nil {
			t.Fatalf("couldn't parse file %q: %v\nFile Contents: %s", file, err, content)
		}

		// Verify file contents.
		if config.Connection.InterfaceName != nic.Name {
			t.Fatalf("NetworkManager configuration %s has unexpected NIC %q, expected %q", file, config.Connection.InterfaceName, nic.Name)
		}
		if config.Connection.ConnType != "ethernet" {
			t.Fatalf("NetworkManager configuration %s has unexpected connection type %q, expected %q", file, config.Connection.ConnType, "ethernet")
		}
		if config.IPv4.Method != "auto" {
			t.Fatalf("NetworkManager configuration %s has unexpected IPv4 method %q, expected %q", file, config.IPv4.Method, "auto")
		}
		if config.IPv6.Method != "auto" {
			t.Fatalf("NetworkManager configuration %s has unexpected IPv6 method %q, expected %q", file, config.IPv6.Method, "auto")
		}
	}
}
