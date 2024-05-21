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

// Package packageupgrade tests that the guest environment and other
// necessary packages are installed and configured correctly.
package packageupgrade

import (
	imagetest "github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "packageupgrade"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	// These tests are against googet which is only used on Windows
	if utils.HasFeature(t.Image, "WINDOWS") {
		vm1, err := t.CreateTestVM("installDrivers")
		if err != nil {
			return err
		}
		vm1.RunTests("TestDriverUpgrade")

		vm2, err := t.CreateTestVM("installPackages")
		if err != nil {
			return err
		}
		vm2.RunTests("TestPackageUpgrade")
	}
	return nil
}
