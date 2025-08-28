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

package nicsetup

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/nicsetup/managers"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	// instanceConfigLoc is the location of the instance config file to modify.
	// .template takes priority over the non-template files. This file shouldn't
	// exist by default on any image.
	instanceConfigLoc = "/etc/default/instance_configs.cfg.template"
)

var (
	// supportsIpv6 is whether the image supports IPv6. If not, then all IPv6-related tests will be skipped.
	supportsIpv6 bool
)

// getCurrentTime returns the current time in RFC3339 format.
func getCurrentTime() string {
	return time.Now().Format(time.RFC3339)
}

// getNumInterfaces returns the number of interfaces set by the setup.
func getNumInterfaces(t *testing.T) int {
	t.Helper()
	t.Logf("%s: Getting number of interfaces", getCurrentTime())

	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces() failed: %v", err)
	}
	return len(utils.FilterLoopbackTunnelingInterfaces(ifaces))
}

// getSupportsIPv6 sets the global variable supportsIpv6 to whether the image supports IPv6.
func getSupportsIPv6(t *testing.T) {
	t.Helper()
	var err error

	val, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", supportIpv6Key)
	if err != nil {
		t.Fatalf("couldn't get support-ipv6 from metadata: %v", err)
	}
	supportsIpv6, err = strconv.ParseBool(val)
	if err != nil {
		t.Fatalf("couldn't convert support-ipv6 to bool: %v", err)
	}
}

// enablePrimaryNIC enables/disables the primary NIC configuration.
func enablePrimaryNIC(t *testing.T, enable bool) {
	t.Helper()
	enablePrimaryNICConfig := fmt.Sprintf(`[NetworkInterfaces]
manage_primary_nic = %t
`, enable)

	// Write the instance config file.
	if err := os.WriteFile(instanceConfigLoc, []byte(enablePrimaryNICConfig), 0644); err != nil {
		t.Fatalf("couldn't write to instance config file: %v", err)
	}

	// Restart the guest agent.
	if err := utils.RestartAgent(utils.Context(t)); err != nil {
		t.Fatalf("couldn't restart guest agent: %v", err)
	}

	// Wait for the guest agent to restart.
	time.Sleep(time.Second * 20)
}

// TestNICSetup tests the NIC setup for the primary NIC, and the secondary NIC
// if it exists.
func TestNICSetup(t *testing.T) {
	t.Logf("%s: Testing Primary NIC", getCurrentTime())
	primaryNIC := managers.GetNIC(t, 0)
	var secondaryNIC managers.EthernetInterface

	numInterfaces := getNumInterfaces(t)
	t.Logf("%s: Number of interfaces: %d", getCurrentTime(), numInterfaces)

	if numInterfaces > 1 {
		secondaryNIC = managers.GetNIC(t, 1)
	}
	getSupportsIPv6(t)

	isUbuntu1804 := managers.IsUbuntu1804(t)

	// Check that no configurations for the primary NIC exist.
	managers.VerifyNIC(t, primaryNIC, false)

	// Enable primary NIC configuration.
	enablePrimaryNIC(t, true)
	t.Logf("%s: Enabled primary NIC configuration", getCurrentTime())

	shouldExist := true
	if isUbuntu1804 {
		// Agent does not launch dhclient on Ubuntu 18.04 for primary NIC.
		shouldExist = false
	}

	// Check the configurations for the primary NIC exist.
	managers.VerifyNIC(t, primaryNIC, shouldExist)

	// Check that the primary NIC has the proper connection.
	verifyConnection(t, primaryNIC)

	// Disable primary NIC configuration.
	enablePrimaryNIC(t, false)
	t.Logf("%s: Disabled primary NIC configuration", getCurrentTime())

	// Check that the configurations for the primary NIC don't exist.
	managers.VerifyNIC(t, primaryNIC, false)
	t.Logf("%s: Finished testing primary NIC", getCurrentTime())

	// Test secondary NIC if it exists.
	if numInterfaces < 2 {
		t.Logf("%s: No secondary NIC, skipping secondary NIC test", getCurrentTime())
		return
	}
	t.Logf("%s: Testing secondary NIC", getCurrentTime())

	// Check the configurations for the secondary NIC exist.
	managers.VerifyNIC(t, secondaryNIC, true)

	// Check that the secondary NIC has the proper connection.
	verifyConnection(t, secondaryNIC)

	t.Logf("%s: Finished testing secondary NIC", getCurrentTime())
}

// TestEmpty is a no-op test that sets up the ping VM and sets the IPv6 address
// in metadata.
func TestEmpty(t *testing.T) {
	getSupportsIPv6(t)
	testEmpty(t)
}
