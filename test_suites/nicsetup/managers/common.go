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

// Package managers provides test helpers for checking the configurations for
// NIC managers.
package managers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/exceptions"
)

// NicStackType represents the type of a NIC. It can be IPv4-only, IPv4/IPv6, or
// IPv6-only.
type NicStackType int

const (
	// Ipv4 represents an IPv4-only NIC.
	Ipv4 NicStackType = 0x1
	// Ipv4Ipv6 represents an IPv4/IPv6 NIC.
	Ipv4Ipv6 NicStackType = 0x3
	// Ipv6 represents an IPv6-only NIC.
	Ipv6 NicStackType = 0x2
)

var (
	// vmNameOnce is used to log the VM name only once.
	vmNameOnce sync.Once
	// image is the image of the instance.
	image string
)

// EthernetInterface represents an ethernet interface.
type EthernetInterface struct {
	// Name is the name of the interface.
	Name string
	// Index is the index of the interface.
	Index int
	// StackType is the stack type of the NIC.
	StackType NicStackType
	// IPv4Address is the IPv4 address of the interface.
	IPv4Address string
	// IPv6Address is the IPv6 address of the interface.
	IPv6Address string
}

// nicManager represents the NIC manager used by the guest agent.
type nicManager int

const (
	// systemdNetworkd represents the systemd-networkd NIC manager.
	systemdNetworkd nicManager = iota
	// dhclient represents the dhclient NIC manager.
	dhclient
	// netplan represents the netplan NIC manager.
	netplan
	// networkManager represents the NetworkManager NIC manager.
	networkManager
	// wicked represents the wicked NIC manager.
	wicked
)

func init() {
	var err error
	image, err = utils.GetMetadata(context.Background(), "instance", "image")
	if err != nil {
		fmt.Printf("couldn't get image from metadata: %v\n", err)
	}
}

// VerifyNIC verifies whether the configurations for the given NIC and
// network manager service exists or not.
func VerifyNIC(t *testing.T, nic EthernetInterface, exist bool) {
	t.Helper()

	mgr := primaryNICManager(t)
	currTime := time.Now().Format(time.RFC3339)

	switch mgr {
	case systemdNetworkd:
		t.Logf("%s: Testing systemd-networkd for NIC %q", currTime, nic.Name)
		testSystemdNetworkd(t, nic, exist)
	case dhclient:
		t.Logf("%s: Testing dhclient for NIC %q", currTime, nic.Name)
		testDhclient(t, nic, exist)
	case netplan:
		t.Logf("%s: Testing netplan for NIC %q", currTime, nic.Name)
		testNetplan(t, nic, exist)
	case networkManager:
		t.Logf("%s: Testing NetworkManager for NIC %q", currTime, nic.Name)
		testNetworkManager(t, nic, exist)
	case wicked:
		t.Logf("%s: Testing wicked for NIC %q", currTime, nic.Name)
		testWicked(t, nic, exist)
	default:
		t.Fatalf("unknown nic manager: %d", mgr)
	}
}

// GetNIC returns the NIC with the given index.
func GetNIC(t *testing.T, index int) EthernetInterface {
	t.Helper()
	nicIface, err := utils.GetInterface(utils.Context(t), index)
	if err != nil {
		t.Fatalf("couldn't get interface: %v", err)
	}
	return EthernetInterface{
		Name:      nicIface.Name,
		Index:     index,
		StackType: getNICType(t, index),
	}
}

// getNICType returns the network stack type of the NIC with the given index.
func getNICType(t *testing.T, index int) NicStackType {
	t.Helper()
	name, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "_test_vmname")
	if err != nil {
		t.Fatalf("couldn't get _test_vmname from metadata: %v", err)
	}

	currTime := time.Now().Format(time.RFC3339)
	// Log the VM name for debugging purposes. We can't include the VM name in
	// the test name, so we log it to give clarity to the test results. We only
	// log the VM name once to avoid spamming the logs.
	vmNameOnce.Do(func() { t.Logf("%s: VM name: %s\n", currTime, name) })

	// The VM name is of the form <network stack type><network stack type>...
	// For example, "ipv4ipv4" is a multiNIC VM with two IPv4 NICs. The network
	// stack type should be 4 characters long.
	start := index * 4
	typeName := name[start : start+4]
	switch typeName {
	case "ipv4":
		return Ipv4
	case "dual":
		return Ipv4Ipv6
	case "ipv6":
		return Ipv6
	default:
		t.Fatalf("unknown network stack type: %s", typeName)
	}
	return Ipv4
}

func isUbuntu1804(t *testing.T) bool {
	t.Helper()
	return exceptions.MatchAll(image, exceptions.ImageUbuntu, exceptions.Exception{
		Version: 1804,
		Type:    exceptions.Equal,
	})
}

// primaryNICManager returns the primary NIC manager.
//
// This is a very basic check, and may have mixed results on images with multiple
// NIC managers.
func primaryNICManager(t *testing.T) nicManager {
	t.Helper()
	t.Logf("Checking primary NIC manager")

	// Ubuntu 18.04 is a special case where netplan is present, but we don't use it.
	if !isUbuntu1804(t) {
		// Check for netplan
		if _, err := exec.LookPath("netplan"); err == nil {
			return netplan
		}
		// Check systemd-networkd.
		if _, err := exec.Command("systemctl", "is-active", "systemd-networkd").Output(); err == nil {
			return systemdNetworkd
		}
	}
	// Check NetworkManager.
	if _, err := exec.Command("systemctl", "is-active", "NetworkManager").Output(); err == nil {
		return networkManager
	}
	// Check wicked.
	if _, err := exec.Command("wicked", "--version").Output(); err == nil {
		return wicked
	}
	// Default to dhclient.
	return dhclient
}

// verifyFileExists checks if a file exists or not.
func verifyFileExists(t *testing.T, path string, exist bool) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if exist {
				t.Errorf("file %q does not exist", path)
			}
			return
		}
		t.Fatalf("couldn't stat file %q: %v", path, err)
	}
	if !exist {
		t.Errorf("file %q exists, but shouldn't", path)
	}
}
