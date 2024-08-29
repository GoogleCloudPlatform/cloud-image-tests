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
    "flag"
    "fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "imageboot"

var testExcludeFilter = flag.String("imageboot_test_exclude_filter", "", "Regex filter that excludes imageboot test cases. Only cases with a matching test name will be skipped.")

var sbUnsupported = []*regexp.Regexp{
	// Permanent exceptions
	regexp.MustCompile("debian-1[01].*arm64"),
	regexp.MustCompile("windows-server-2012-r2-dc-core"), // Working but not easily testable and EOL in 1.5 months
	// Temporary exceptions
	// Waiting on MSFT signed shims:
	regexp.MustCompile("rhel-9.*arm64"),                  // https://issues.redhat.com/browse/RHEL-4326
	regexp.MustCompile("(sles-15|opensuse-leap).*arm64"), // https://bugzilla.suse.com/show_bug.cgi?id=1214761
}

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	exfilter, err := regexp.Compile(*testExcludeFilter)
	if err != nil {
		return fmt.Errorf("Invalid test case exclude filter: %v", err)
	}
    if exfilter.MatchString("TestGuestBoot") && exfilter.MatchString("TestGuestReboot") {
        // Skip VM creation & save resource if no tests are being run on vm `boot`
        fmt.Println("Skipping tests 'TestGuestBoot|TestGuestReboot'")
    } else {
        vm, err := t.CreateTestVM("boot")
        if err != nil {
            return err
        }
        if err := vm.Reboot(); err != nil {
            return err
        }
        if exfilter.MatchString("TestGuestBoot") {
            fmt.Println("Skipping test 'TestGuestBoot'")
        } else {
            vm.RunTests("TestGuestBoot")
        }
        if exfilter.MatchString("TestGuestReboot") {
            fmt.Println("Skipping test 'TestGuestReboot'")
        } else {
            vm.RunTests("TestGuestReboot")
        }
	}
    if exfilter.MatchString("TestGuestRebootOnHost") {
        fmt.Println("Skipping test 'TestGuestRebootOnHost'")
    } else {
        vm2, err := t.CreateTestVM("guestreboot")
        if err != nil {
            return err
        }
        vm2.RunTests("TestGuestRebootOnHost")
	}
    if exfilter.MatchString("TestStartTime") && exfilter.MatchString("TestBootTime") {
        // Skip VM creation & save resource if no tests are being run on vm `boottime`
        fmt.Println("Skipping tests 'TestStartTime|TestBootTime'")
    } else {
        vm3, err := t.CreateTestVM("boottime")
        if err != nil {
            return err
        }
        vm3.AddMetadata("start-time", strconv.Itoa(time.Now().Second()))
        if exfilter.MatchString("TestStartTime") {
            fmt.Println("Skipping tests 'TestStartTime'")
        } else {
            vm3.RunTests("TestStartTime")
        }

        if exfilter.MatchString("TestBootTime") {
            fmt.Println("Skipping tests 'TestBootTime'")
        } else {
            vm3.RunTests("TestBootTime")
        }
	}

	for _, r := range sbUnsupported {
		if r.MatchString(t.Image.Name) {
			return nil
		}
	}
	if !utils.HasFeature(t.Image, "UEFI_COMPATIBLE") {
		return nil
	}
    if exfilter.MatchString("TestGuestSecureBoot") {
        fmt.Println("Skipping tests 'TestGuestSecureBoot'")
    } else {
        vm4, err := t.CreateTestVM("secureboot")
        if err != nil {
            return err
        }
        vm4.EnableSecureBoot()
        vm4.RunTests("TestGuestSecureBoot")
    }
	return nil
}
