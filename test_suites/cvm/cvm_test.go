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

package cvm

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

var sevMsgList = []string{"AMD Secure Encrypted Virtualization (SEV) active", "AMD Memory Encryption Features active: SEV", "Memory Encryption Features active: AMD SEV"}
var sevSnpMsgList = []string{"SEV: SNP guest platform device initialized", "Memory Encryption Features active: SEV SEV-ES SEV-SNP", "Memory Encryption Features active: AMD SEV SEV-ES SEV-SNP"}
var tdxMsgList = []string{"Memory Encryption Features active: TDX", "Memory Encryption Features active: Intel TDX"}

func searchDmesg(t *testing.T, matches []string) {
	output, err := exec.Command("dmesg").CombinedOutput()
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	for _, m := range matches {
		if strings.Contains(string(output), m) {
			return
		}
	}
	t.Fatal("CPU feature not active or found")
}

func findGuestDevice(t *testing.T, devicepath string) {
	_, err := os.Stat(devicepath)
	if err != nil {
		t.Fatalf("could not find guest device: %v", err)
	}
}

func TestSEVEnabled(t *testing.T) {
	searchDmesg(t, sevMsgList)
}

func TestSEVSNPEnabled(t *testing.T) {
	searchDmesg(t, sevSnpMsgList)
	if !utils.IsWindows() {
		findGuestDevice(t, "/dev/sev-guest")
	}
}

func TestTDXEnabled(t *testing.T) {
	searchDmesg(t, tdxMsgList)
	if !utils.IsWindows() {
		findGuestDevice(t, "/dev/tdx-guest")
	}
}

func TestLiveMigrate(t *testing.T) {
	marker := "/var/lm-test-start"
	if utils.IsWindows() {
		marker = `C:\lm-test-start`
	}
	if _, err := os.Stat(marker); err != nil && !os.IsNotExist(err) {
		t.Fatalf("could not determine if live migrate testing has already started: %v", err)
	} else if err == nil {
		t.Fatal("unexpected reboot during live migrate test")
	}
	err := os.WriteFile(marker, nil, 0777)
	if err != nil {
		t.Fatalf("could not mark beginning of live migrate testing: %v", err)
	}
	ctx := utils.Context(t)
	prj, zone, err := utils.GetProjectZone(ctx)
	if err != nil {
		t.Fatalf("could not find project and zone: %v", err)
	}
	inst, err := utils.GetInstanceName(ctx)
	if err != nil {
		t.Fatalf("could not get instance: %v", err)
	}
	client, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		t.Fatalf("could not make compute api client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	req := &computepb.SimulateMaintenanceEventInstanceRequest{
		Project:  prj,
		Zone:     zone,
		Instance: inst,
	}
	op, err := client.SimulateMaintenanceEvent(ctx, req)
	if err != nil {
		t.Fatalf("could not migrate self: %v", err)
	}
	op.Wait(ctx) // Errors here come from things completely out of our control, such as the availability of a physical machine to take our VM.
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("could not confirm migrate testing has started ok: %v", err)
	}
	_, err = http.Get("https://cloud.google.com/")
	if err != nil {
		t.Errorf("lost network connection after live migration")
	}
}
