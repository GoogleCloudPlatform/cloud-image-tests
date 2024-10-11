// Copyright 2024 Google LLC.
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

package network

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	networkctlCmd = "networkctl"
	nmcliCmd      = "nmcli"
	wickedCmd     = "wicked"
)

// TestDHCP test secondary interfaces are configured with a single dhclient process.
func TestDHCP(t *testing.T) {
	var cmd *exec.Cmd
	var err error

	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("could not get image name: %s", err)
	}

	if strings.Contains(image, "debian-10") || strings.Contains(image, "debian-11") || strings.Contains(image, "debian-12") {
		t.Skipf("DHCP test not supported on: %s", image)
	}

	// Run every case: if one command or check succeeds, the test passes.
	if utils.IsWindows() {
		checkDhcpWindows(t)
		return
	}

	if utils.CheckLinuxCmdExists(networkctlCmd) {
		cmd = exec.Command(networkctlCmd, "status")
		if err = parseNetworkctlOutput(cmd); err == nil {
			return
		}
	}

	if utils.CheckLinuxCmdExists(nmcliCmd) {
		cmd = exec.Command(nmcliCmd, "device", "show")
		if err = parseNmcliOutput(cmd); err == nil {
			return
		}
	}

	if utils.CheckLinuxCmdExists(wickedCmd) {
		cmd = exec.Command(wickedCmd, "show", "all")
		if err = parseWickedOutput(cmd); err == nil {
			return
		}
	}

	// Base dhcp case for debian 10, debian 11, ubuntu 16, etc.
	if err = checkDHCPProcess(t); err != nil {
		t.Fatalf("did not find dhcp process: %v", err)
	}

	if err != nil {
		t.Fatalf("dhcp command failed or not found: %v", err)
	}
}

func checkDhcpWindows(t *testing.T) {
	ifaceIndexes, err := utils.GetMetadata(utils.Context(t), "instance", "network-interfaces")
	if err != nil {
		t.Errorf("could not get interfaces: %s", err)
	}
	for _, ifaceIndex := range strings.Split(ifaceIndexes, "\n") {
		if ifaceIndex == "" {
			continue
		}
		i, err := strconv.Atoi(strings.TrimSuffix(ifaceIndex, "/"))
		if err != nil {
			t.Errorf("can't convert %s to int", ifaceIndex)
		}
		iface, err := utils.GetInterface(utils.Context(t), i)
		if err != nil {
			t.Errorf("could not find interface %d", i)
		}
		cmd := fmt.Sprintf(`Get-NetIPInterface -InterfaceAlias "%s" -Dhcp Enabled`, iface.Name)
		out, err := utils.RunPowershellCmd(cmd)
		if err != nil {
			t.Errorf("could not verify that iface %s used DHCP to obtain an IP address: %s\ncmd %s returned %q", iface.Name, err, cmd, out.Stdout)
		}
	}
}

func parseNetworkctlOutput(cmd *exec.Cmd) error {
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("networkctl command failed %v", err)
	}

	// check for any line with dhcpv4. If the line is found, check that
	// a valid IP address is found in the same line.
	for _, line := range strings.Split(string(out), "\n") {
		upperLine := strings.ToUpper(line)
		if strings.Contains(upperLine, "DHCPV4") {
			for _, token := range strings.Fields(upperLine) {
				if validIPOrCIDR(token) {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("valid ip4 address not found in networkctl output")
}

func parseNmcliOutput(cmd *exec.Cmd) error {
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("nmcli command failed %v", err)
	}

	// check for any line with dhcpv4. If the line is found, check that
	// a valid IP address is found in the same line.
	for _, line := range strings.Split(string(out), "\n") {
		upperLine := strings.ToUpper(line)
		if strings.Contains(upperLine, "IP4.ADDRESS") {
			for _, token := range strings.Fields(upperLine) {
				if validIPOrCIDR(token) {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("IPV4Address not found in nmcli output")
}

func parseWickedOutput(cmd *exec.Cmd) error {
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("wicked command failed %v", err)
	}

	for _, line := range strings.Split(string(out), "\n") {
		upperLine := strings.ToUpper(line)
		if strings.Contains(upperLine, "IPV4") && strings.Contains(upperLine, "DHCP") {
			for _, token := range strings.Fields(upperLine) {
				if validIPOrCIDR(token) {
					return nil
				}
			}
		}
	}

	return fmt.Errorf("dhcpv4 or ip address not found in wicked output")
}

func checkDHCPProcess(t *testing.T) error {
	iface, err := utils.GetInterface(utils.Context(t), 1)
	if err != nil {
		return fmt.Errorf("couldn't get secondary interface: %v", err)
	}

	cmd := exec.Command("ps", "x")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("ps command failed %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "dhclient") && strings.Contains(line, iface.Name) {
			return nil
		}
	}
	return fmt.Errorf("failed finding dhclient process")
}

// accepts IP addresses in the form of a.b.c.d or a.b.c.d/IPNET
func validIPOrCIDR(token string) bool {
	if IPAddress := net.ParseIP(token); IPAddress != nil {
		return true
	}

	IPAddress, _, err := net.ParseCIDR(token)
	if IPAddress != nil && err == nil {
		return true
	}

	return false
}
