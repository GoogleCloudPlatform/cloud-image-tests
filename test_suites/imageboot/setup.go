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
	"strconv"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "imageboot"

var sbUnsupported = []*regexp.Regexp{
	// Permanent exceptions
	regexp.MustCompile("debian-1[01].*arm64"),
	regexp.MustCompile("windows-server-2012-r2-dc-core"), // Working but not easily testable and EOL in 1.5 months
	// Temporary exceptions
	// Waiting on MSFT signed shims:
	regexp.MustCompile("rocky-linux-9.*arm64"),           // https://bugs.rockylinux.org/view.php?id=4027
	regexp.MustCompile("rhel-9.*arm64"),                  // https://issues.redhat.com/browse/RHEL-4326
	regexp.MustCompile("(sles-15|opensuse-leap).*arm64"), // https://bugzilla.suse.com/show_bug.cgi?id=1214761
}

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	vm, err := t.CreateTestVM("boot")
	if err != nil {
		return err
	}
	if err := vm.Reboot(); err != nil {
		return err
	}
	vm.RunTests("TestGuestBoot|TestGuestReboot$")

	vm2, err := t.CreateTestVM("guestreboot")
	if err != nil {
		return err
	}
	vm2.RunTests("TestGuestRebootOnHost")

	vm3, err := t.CreateTestVM("boottime")
	if err != nil {
		return err
	}
	vm3.AddMetadata("start-time", strconv.Itoa(time.Now().Second()))
	vm3.RunTests("TestStartTime|TestBootTime")

	for _, r := range sbUnsupported {
		if r.MatchString(t.Image.Name) {
			return nil
		}
	}
	if !utils.HasFeature(t.Image, "UEFI_COMPATIBLE") {
		return nil
	}
	vm4, err := t.CreateTestVM("secureboot")
	if err != nil {
		return err
	}
	vm4.EnableSecureBoot()
	vm4.RunTests("TestGuestSecureBoot")
	return nil
}
