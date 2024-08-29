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

// Package hostnamevalidation is a CIT suite for testing custom hostnames.
package hostnamevalidation

import (
    "flag"
    "fmt"
    "regexp"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "hostnamevalidation"

var testExcludeFilter = flag.String("hostnamevalidation_test_exclude_filter", "", "Regex filter that excludes hostnamevalidation test cases. Only cases with a matching test name will be skipped.")

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	exfilter, err := regexp.Compile(*testExcludeFilter)
	if err != nil {
		return fmt.Errorf("Invalid test case exclude filter: %v", err)
	}
	vm1, err := t.CreateTestVM("vm1")
	if err != nil {
		return err
	}
    if exfilter.MatchString("TestHostname") {
        fmt.Println("Skipping test 'TestHostname'")
    } else {
	    vm1.RunTests("TestHostname")
	}
    if exfilter.MatchString("TestFQDN") {
        fmt.Println("Skipping test 'TestFQDN'")
    } else {
	    vm1.RunTests("TestFQDN")
	}
    if exfilter.MatchString("TestHostKeysGeneratedOnce") {
        fmt.Println("Skipping test 'TestHostKeysGeneratedOnce'")
    } else {
	    vm1.RunTests("TestHostKeysGeneratedOnce")
	}
    if exfilter.MatchString("TestHostsFile") {
        fmt.Println("Skipping test 'TestHostsFile'")
    } else {
	    vm1.RunTests("TestHostsFile")
	}

	// custom host name test not yet implemented for windows
	if !utils.HasFeature(t.Image, "WINDOWS") {
        if exfilter.MatchString("TestCustomHostname") {
            fmt.Println("Skipping test 'TestCustomHostname'")
        } else {
            vm2, err := t.CreateTestVM("vm2.custom.domain")
            if err != nil {
                return err
            }
            vm2.RunTests("TestCustomHostname")
        }
	}

	return nil
}
