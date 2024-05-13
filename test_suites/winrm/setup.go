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

// Package winrm tests windows remote management functionality.
package winrm

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "winrm"

const user = "test-user"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if !utils.HasFeature(t.Image, "WINDOWS") {
		return nil
	}
	passwd := utils.ValidWindowsPassword(14)

	vm, err := t.CreateTestVM("client")
	if err != nil {
		return err
	}
	vm.AddMetadata("winrm-passwd", passwd)
	vm.RunTests("TestWinrmConnection")

	vm2, err := t.CreateTestVM("server")
	if err != nil {
		return err
	}
	vm2.AddMetadata("winrm-passwd", passwd)
	vm2.RunTests("TestWaitForWinrmConnection")

	return nil
}
