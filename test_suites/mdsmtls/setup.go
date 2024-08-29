// Copyright 2023 Google LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     https://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package mdsmtls is a CIT suite for testing mtls communication with the mds.
package mdsmtls

import (
    "flag"
    "fmt"
    "regexp"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
const Name = "mdsmtls"

var testExcludeFilter = flag.String("mdsmtls_test_exclude_filter", "", "Regex filter that excludes mdsmtls test cases. Only cases with a matching test name will be skipped.")

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	exfilter, err := regexp.Compile(*testExcludeFilter)
	if err != nil {
		return fmt.Errorf("Invalid test case exclude filter: %v", err)
	}
	if !utils.HasFeature(t.Image, "UEFI_COMPATIBLE") {
		return nil
	}
    if exfilter.MatchString("TestMTLSCredsExists") && exfilter.MatchString("TestMTLSJobScheduled") {
        // Skip VM creation & save resource if no tests are being run on `vm`
        fmt.Println("Skipping tests 'TestMTLSCredsExists|TestMTLSJobScheduled'")
    } else {
        vm, err := t.CreateTestVM("mtlscreds")
        if err != nil {
            return err
        }
	    if exfilter.MatchString("TestMTLSCredsExists") {
            fmt.Println("Skipping test 'TestMTLSCredsExists'")
        } else {
            vm.RunTests("TestMTLSCredsExists")
        }
        if exfilter.MatchString("TestMTLSJobScheduled") {
            fmt.Println("Skipping test 'TestMTLSJobScheduled'")
        } else {
            vm.RunTests("TestMTLSJobScheduled")
        }
    }
	return nil
}
