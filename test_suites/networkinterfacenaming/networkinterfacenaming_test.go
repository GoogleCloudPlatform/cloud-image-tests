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

package networkinterfacenaming

import (
	"net"
	"regexp"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

var (
	allowedEthNamesImages = []string{
		"cos",
		"debian-11",
		"suse",
		"sles",
		"rhel-8",
		"rhel-9",
		"centos-stream-8",
		"centos-stream-9",
		"almalinux-8",
		"almalinux-9",
		"rocky-linux-8",
		"rocky-linux-9",
	}
	windowsNICNameRegex = regexp.MustCompile("^Ethernet.*")
	ethNICNameRegex     = regexp.MustCompile("^eth[0-9]+")
	// See man systemd.net-naming-scheme
	predictableNICNameRegex = regexp.MustCompile("^en.*")
)

func TestNICNamingScheme(t *testing.T) {
	if utils.IsWindows() {
		nics, err := net.Interfaces()
		if err != nil {
			t.Fatalf("net.Interfaces() = err %v want nil", err)
		}
		for _, nic := range nics {
			if strings.HasPrefix(nic.Name, "Loopback") || strings.HasPrefix(nic.Name, "isatap.") {
				// Ignore loopbacks and ipv6 tunnels
				continue
			}
			if !windowsNICNameRegex.MatchString(nic.Name) {
				t.Errorf("NIC name %q does not match scheme %q", nic.Name, windowsNICNameRegex.String())
			}
		}
		return
	}
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("utils.GetMetadata(ctx, instance, image) = err %v want nil", err)
	}
	nics, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces() = err %v want nil", err)
	}
	for _, nic := range nics {
		if nic.Name == "lo" {
			// Ignore loopback interface
			continue
		}
		if ethNICNameRegex.MatchString(nic.Name) {
			if !canHaveEthNames(image) {
				t.Errorf("Found eth name %q on image %s that is not part of allowlist %v", nic.Name, image, allowedEthNamesImages)
			}
			continue
		}
		if !predictableNICNameRegex.MatchString(nic.Name) {
			t.Errorf("NIC name %q does not match predictable name scheme %q", nic.Name, predictableNICNameRegex.String())
		}
	}
}

func canHaveEthNames(image string) bool {
	for _, alllowedImage := range allowedEthNamesImages {
		if strings.Contains(image, alllowedImage) {
			return true
		}
	}
	return false
}
