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

// Package guestagent is a CIT suite for testing guest agent features.
package guestagent

import (
    "flag"
    "fmt"
    "regexp"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

var testExcludeFilter = flag.String("guestagent_test_exclude_filter", "", "Regex filter that excludes guestagent test cases. Only cases with a matching test name will be skipped.")

// Name is the name of the test package. It must match the directory name.
const Name = "guestagent"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	exfilter, err := regexp.Compile(*testExcludeFilter)
	if err != nil {
		return fmt.Errorf("Invalid test case exclude filter: %v", err)
	}
	diskType := imagetest.PdBalanced
	if strings.HasPrefix(t.MachineType.Name, "n4-") {
		diskType = imagetest.HyperdiskBalanced
	}
    if exfilter.MatchString("TestTelemetryDisabled") {
        fmt.Println("Skipping test 'TestTelemetryDisabled'")
    } else {
        telemetrydisabledinst := &daisy.Instance{}
        telemetrydisabledinst.Scopes = []string{"https://www.googleapis.com/auth/cloud-platform"}
        telemetrydisabledinst.Name = "telemetryDisabled"
        telemetrydisabledvm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: telemetrydisabledinst.Name, Type: diskType}}, telemetrydisabledinst)
        if err != nil {
            return err
        }
        telemetrydisabledvm.AddMetadata("disable-guest-telemetry", "true")
        telemetrydisabledvm.RunTests("TestTelemetryDisabled")
	}
    if exfilter.MatchString("TestTelemetryEnabled") {
        fmt.Println("Skipping test 'TestTelemetryEnabled'")
    } else {
        telemetryenabledinst := &daisy.Instance{}
        telemetryenabledinst.Scopes = []string{"https://www.googleapis.com/auth/cloud-platform"}
        telemetryenabledinst.Name = "telemetryEnabled"
        telemetryenabledvm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: telemetryenabledinst.Name, Type: diskType}}, telemetryenabledinst)
        if err != nil {
            return err
        }
        telemetryenabledvm.AddMetadata("disable-guest-telemetry", "false")
        telemetryenabledvm.RunTests("TestTelemetryEnabled")
	}
    if exfilter.MatchString("TestSnapshotScripts") {
        fmt.Println("Skipping test 'TestSnapshotScripts'")
    } else {
        snapshotinst := &daisy.Instance{}
        snapshotinst.Scopes = []string{"https://www.googleapis.com/auth/cloud-platform"}
        snapshotinst.Name = "snapshotScripts"
        snapshotvm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: snapshotinst.Name, Type: diskType}}, snapshotinst)
        if err != nil {
            return err
        }
        snapshotvm.RunTests("TestSnapshotScripts")
    }
	if utils.HasFeature(t.Image, "WINDOWS") {
        if exfilter.MatchString("TestWindowsPasswordReset") {
            fmt.Println("Skipping test 'TestWindowsPasswordReset'")
        } else {
            passwordInst := &daisy.Instance{}
            passwordInst.Scopes = append(passwordInst.Scopes, "https://www.googleapis.com/auth/cloud-platform")
            windowsaccountVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "windowsaccount"}}, passwordInst)
            if err != nil {
                return err
            }
            windowsaccountVM.RunTests("TestWindowsPasswordReset")
        }
	}
	return nil
}
