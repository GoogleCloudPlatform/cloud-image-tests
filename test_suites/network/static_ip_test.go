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
	"math/bits"
	"strconv"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Test that interfaces are configured with the ip set by the GCE control plane
func TestStaticIP(t *testing.T) {
	ctx := utils.Context(t)
	ifaceIndexes, err := utils.GetMetadata(ctx, "instance", "network-interfaces")
	if err != nil {
		t.Errorf("could not get interfaces: %s", err)
	}
	for _, ifaceIndex := range strings.Split(ifaceIndexes, "\n") {
		ifaceIndex = strings.TrimSuffix(ifaceIndex, "/")
		if ifaceIndex == "" {
			continue
		}
		expectedIP, err := utils.GetMetadata(ctx, "instance", "network-interfaces", ifaceIndex, "ip")
		if err != nil {
			t.Errorf("could not get expected IP for interface %s: %v", ifaceIndex, err)
		}
		mask, err := utils.GetMetadata(ctx, "instance", "network-interfaces", ifaceIndex, "subnetmask")
		if err != nil {
			t.Errorf("could not get subnet mask for interface %s: %v", ifaceIndex, err)
		}
		if ifaceIndex == "0" && utils.IsWindows() {
			// TODO (acrate): check subnet on secondary interfaces (pending guest-agent fixes)
			// TODO (acrate): check subnet on linux (pending GCE)
			expectedIP += suffixFromMask(mask)
		}
		mac, err := utils.GetMetadata(ctx, "instance", "network-interfaces", ifaceIndex, "mac")
		if err != nil {
			t.Errorf("could not get interface %s mac address: %v", ifaceIndex, err)
			continue
		}
		iface, err := utils.GetInterfaceByMAC(mac)
		if err != nil {
			t.Errorf("could not get interface index %s with mac %s: %v", ifaceIndex, mac, err)
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			t.Errorf("could not get addrs from interface %s: %v", ifaceIndex, err)
		}
		var ok bool
		for _, addr := range addrs {
			t.Logf("found addr %s on interface %s", addr, ifaceIndex)
			if strings.HasPrefix(addr.String(), expectedIP) {
				ok = true
				break
			}
		}
		if ok {
			continue
		}
		t.Errorf("no address for interface %s with ip %s was found", ifaceIndex, expectedIP)
	}
}

func suffixFromMask(mask string) string {
	var sum int
	for _, n := range strings.Split(mask, ".") {
		i, err := strconv.ParseUint(n, 10, 8)
		if err == nil {
			sum += bits.OnesCount64(i)
		}
	}
	return fmt.Sprintf("/%d", sum)
}
