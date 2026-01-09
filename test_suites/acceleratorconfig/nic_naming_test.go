// Copyright 2025 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package acceleratorconfig

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

var (
	// See man systemd.net-naming-scheme
	predictableNICNameRegex = regexp.MustCompile("^en.*")
	rdmaNICNameRegex        = regexp.MustCompile(`^gpu[0-9]+rdma[0-9]+$`)
	pciBusRegex             = regexp.MustCompile(`:[0-9A-Fa-f]{2}:`)
)

// Each instance should have two GVNICs named as a predictable interface name,
// followed by 8 CX-7 NICs named after the nearest GPU.
//
// PCIe topology should be structured like this:
//
// switch
// |-> gpu
// |-> CX-7 NIC
//
// Test that for each CX-7 NIC, it's named after the GPU on the same PCI switch.
func TestNICNaming(t *testing.T) {
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("utils.GetMetadata(ctx, instance, image) = err %v want nil", err)
	}
	for i := 0; i < 2; i++ {
		mac, err := utils.GetMetadata(ctx, "instance", "network-interfaces", fmt.Sprintf("%d", i), "mac")
		if err != nil {
			t.Fatalf("utils.GetMetadata(ctx, instance, network-interfaces, %d, mac) = err %v", i, err)
		}
		iface, err := utils.GetInterfaceByMAC(mac)
		if err != nil {
			continue
		}
		if !predictableNICNameRegex.MatchString(iface.Name) {
			t.Errorf("NIC name %q does not match predictable name scheme %q", iface.Name, predictableNICNameRegex.String())
		}
	}
	for i := 2; i < 10; i++ {
		mac, err := utils.GetMetadata(ctx, "instance", "network-interfaces", fmt.Sprintf("%d", i), "mac")
		if err != nil {
			t.Fatalf("utils.GetMetadata(ctx, instance, network-interfaces, %d, mac) = err %v", i, err)
		}
		iface, err := utils.GetInterfaceByMAC(mac)
		if err != nil {
			continue
		}
		if !rdmaNICNameRegex.MatchString(iface.Name) {
			// Allow images that don't have intent-based names yet to match the predictable name scheme instead.
			// TODO remove exceptions when predictable name scheme is incorporated in each image.
			if utils.IsUbuntu(image) || utils.IsRocky(image) {
				if !predictableNICNameRegex.MatchString(iface.Name) {
					t.Errorf("NIC name %q does not match predictable name scheme %q", iface.Name, predictableNICNameRegex.String())
				}
			} else {
				t.Errorf("NIC name %q does not match rdma name scheme %q", iface.Name, rdmaNICNameRegex.String())
			}
		}
	}
}
