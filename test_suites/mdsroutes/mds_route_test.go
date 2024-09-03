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

package mdsroutes

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Test that only the primary NIC has a route to the MDS.
func TestMDSRoutes(t *testing.T) {
	ctx := utils.Context(t)
	ifaceIndexes, err := utils.GetMetadata(ctx, "instance", "network-interfaces")
	if err != nil {
		t.Errorf("Could not get interfaces: %s.", err)
	}
	for _, ifaceIndex := range strings.Split(ifaceIndexes, "\n") {
		ifaceIndex = strings.TrimSuffix(ifaceIndex, "/")
		if ifaceIndex == "" {
			continue
		}
		mac, err := utils.GetMetadata(ctx, "instance", "network-interfaces", ifaceIndex, "mac")
		if err != nil {
			t.Errorf("Could not get interface %s mac address: %v.", ifaceIndex, err)
			continue
		}
		iface, err := utils.GetInterfaceByMAC(mac)
		if err != nil {
			t.Errorf("Could not get interface index %s with mac %s: %v.", ifaceIndex, mac, err)
			continue
		}
		if utils.IsWindows() {
			out, err := utils.RunPowershellCmd(fmt.Sprintf(`(Get-NetRoute -InterfaceAlias %s).DestinationPrefix`, iface.Name))
			if err != nil {
				t.Fatalf("utils.RunPowershellCmd(%q) failed: %v", fmt.Sprintf(`(Get-NetRoute -InterfaceAlias %s).DestinationPrefix`, iface.Name), err)
			}
			if strings.Contains(out.Stdout, "169.254.169.254/") != (ifaceIndex == "0") {
				t.Errorf("Want route to 169.254.169.254 only on NIC 0, found route to MDS = %t on NIC %s.", strings.Contains(out.Stdout, "169.254.169.254/"), ifaceIndex)
				t.Logf("Route destinations for NIC %s: %s", ifaceIndex, out.Stdout)
			}
		} else {
			out, err := exec.CommandContext(ctx, "ip", "route", "show", "dev", iface.Name).Output()
			if err != nil {
				t.Fatalf("exec.CommandContext(ctx, %q) failed: %v", fmt.Sprintf("ip route show dev %s", iface.Name), err)
			}
			var hasMDSRoute bool
			for _, line := range strings.Split(string(out), "\n") {
				f := strings.Fields(line)
				if len(f) < 1 {
					continue
				}
				hasMDSRoute = (f[0] == "169.254.169.254")
				if hasMDSRoute {
					break
				}
			}
			if hasMDSRoute != (ifaceIndex == "0") {
				t.Errorf("Want route to 169.254.169.254 only on NIC 0, found route to MDS = %t on NIC %s.", hasMDSRoute, ifaceIndex)
				t.Logf("Route table for NIC %s: %s", ifaceIndex, out)
			}
		}
	}
}
