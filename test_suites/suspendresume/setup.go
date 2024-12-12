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
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

var (
	// Name is the name of the test package. It must match the directory name.
	Name = "suspendresume"
	// unsupportedImages is a list of images which do not support suspend resume. These strings will be match against the name field.
	unsupportedImages = []string{"windows-server-2025", "windows-2025-dc", "windows-11-24h2", "windows-server-2012-r2", "rhel-8-2-sap", "rhel-8-1-sap", "debian-10", "ubuntu-pro-1804-bionic-arm64"}
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if t.Image.Architecture == "ARM64" {
		return nil
	}
	for _, match := range unsupportedImages {
		if strings.Contains(t.Image.Name, match) {
			return nil
		}
	}
	suspend := &daisy.Instance{}
	suspend.Scopes = append(suspend.Scopes, "https://www.googleapis.com/auth/cloud-platform")
	suspendvm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "suspend"}}, suspend)
	if err != nil {
		return err
	}
	suspendvm.RunTests("TestSuspend")
	suspendvm.Resume()
	return nil
}
