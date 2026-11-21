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

// Package imageboot is a CIT suite for testing boot, reboot, and secure boot
// functionality.
package imageboot

import (
	"regexp"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "imageboot"

var sbUnsupported = []*regexp.Regexp{
	// Permanent exceptions
	regexp.MustCompile("debian-1[01].*arm64"),
	regexp.MustCompile("windows-server-2012-r2-dc-core"), // Working but not easily testable and EOL in 1.5 months
	regexp.MustCompile("rhel-10-2.*gvnic-baremetal"),     // OOT driver incompatible with Secure Boot policies
	regexp.MustCompile("rhel-10-2.*oot-gve"),             // gVNIC BareMetal duplicate with different name
	// Temporary exceptions
	// Waiting on MSFT signed shims:
	regexp.MustCompile("rhel-8.*arm64"),
	regexp.MustCompile("rhel-9.*arm64"), // https://issues.redhat.com/browse/RHEL-4326
	regexp.MustCompile("rhel-10.*arm64.*"),
	regexp.MustCompile("centos-stream-9.*arm64"),
	regexp.MustCompile("centos-stream-10.*arm64.*"),
	regexp.MustCompile("(sles-15|opensuse-leap).*arm64"), // https://bugzilla.suse.com/show_bug.cgi?id=1214761
}

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	// ShouldRebootDuringTest makes Boot 1 write first-boot-key instead of
	// test-complete so Daisy waits for Boot 2 after vm.Reboot().
	bootInst := &daisy.Instance{
		Metadata: map[string]string{imagetest.ShouldRebootDuringTest: "true"},
	}
	vm, err := t.CreateTestVMFromInstance(bootInst, []*compute.Disk{{Name: "boot"}})
	if err != nil {
		return err
	}
	if err := vm.Reboot(); err != nil {
		return err
	}
	vm.RunTests("^(TestGuestBoot|TestGuestReboot|TestBootTime)$")

	vm2, err := t.CreateTestVM("guestreboot")
	if err != nil {
		return err
	}
	if supportsSecureBoot(t.Image) {
		// TestGuestRebootOnHost reboots on Boot 1; Boot 2 checks both the reboot
		// marker and persistent UEFI Secure Boot state.
		vm2.EnableSecureBoot()
		vm2.RunTests("^(TestGuestRebootOnHost|TestGuestSecureBoot)$")
	} else {
		vm2.RunTests("^TestGuestRebootOnHost$")
	}
	return nil
}

func supportsSecureBoot(image *compute.Image) bool {
	if !utils.HasFeature(image, "UEFI_COMPATIBLE") {
		return false
	}
	for _, r := range sbUnsupported {
		if r.MatchString(image.Name) {
			return false
		}
	}
	return true
}
