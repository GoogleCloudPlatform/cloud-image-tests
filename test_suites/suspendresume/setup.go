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

// Package suspendresume tests suspend and resume functionality.
package suspendresume

import (
    "flag"
    "fmt"
    "regexp"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "suspendresume"

var testExcludeFilter = flag.String("suspendresume_test_exclude_filter", "", "Regex filter that excludes suspendresume test cases. Only cases with a matching test name will be skipped.")

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	exfilter, err := regexp.Compile(*testExcludeFilter)
	if err != nil {
		return fmt.Errorf("Invalid test case exclude filter: %v", err)
	}
    if exfilter.MatchString("TestSuspend") {
        fmt.Println("Skipping test 'TestSuspend'")
    } else {
        if !strings.Contains(t.Image.Name, "windows-server-2025") && !strings.Contains(t.Image.Name, "windows-11-24h2") && !strings.Contains(t.Image.Name, "rhel-8-2-sap") && !strings.Contains(t.Image.Name, "rhel-8-1-sap") && !strings.Contains(t.Image.Name, "debian-10") && !strings.Contains(t.Image.Family, "ubuntu-pro-1804-lts-arm64") && !strings.Contains(t.Image.Family, "ubuntu-2404-lts") {
            suspend := &daisy.Instance{}
            suspend.Scopes = append(suspend.Scopes, "https://www.googleapis.com/auth/cloud-platform")
            suspendvm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "suspend"}}, suspend)
            if err != nil {
                return err
            }
            suspendvm.RunTests("TestSuspend")
            suspendvm.Resume()
        }
    }
	return nil
}
