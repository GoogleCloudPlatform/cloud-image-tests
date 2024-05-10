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
	"regexp"
	"strconv"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	gceMTU = 1460
)

func TestDefaultMTU(t *testing.T) {
	iface, err := utils.GetInterface(utils.Context(t), 0)
	if err != nil {
		t.Fatalf("couldn't find primary NIC: %v", err)
	}
	if utils.IsWindows() {
		sysprepInstalled, err := utils.RunPowershellCmd(`googet installed google-compute-engine-sysprep.noarch | Select-Object -Index 1`)
		if err != nil {
			t.Fatalf("could not check installed sysprep version: %v", err)
		}
		// YYYYMMDD
		sysprepVerRe := regexp.MustCompile("[0-9]{8}")
		sysprepVer, err := strconv.Atoi(sysprepVerRe.FindString(sysprepInstalled.Stdout))
		if err != nil {
			t.Fatalf("could not determine value of sysprep version: %v", err)
		}
		if sysprepVer <= 20240104 {
			t.Skipf("version %d of gcesysprep is too old to set interface mtu correctly", sysprepVer)
		}
	}
	if iface.MTU != gceMTU {
		t.Fatalf("expected MTU %d on interface %s, got MTU %d", gceMTU, iface.Name, iface.MTU)
	}
}
