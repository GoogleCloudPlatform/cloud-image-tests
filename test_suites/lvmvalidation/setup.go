// Copyright 2025 Google LLC.
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

// Package lvmvalidation tests that the LVM layout is correct and functional.
package lvmvalidation

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test suite.
var Name = "lvmvalidation"

// TestSetup sets up the test workflow.
// For LVM images it should test that LVM is present and the partitions are correct.
// For non-lvm it should ensure that LVM isn't installed and that the partitions are normal.
func TestSetup(t *imagetest.TestWorkflow) error {
	// skip if not rhel image
	if !utils.IsRHEL(t.Image.Name) {
		t.Skip("LVM validation test only supports Red Hat images.")
	}

	vm, err := t.CreateTestVM("lvmTest")
	if err != nil {
		return err
	}
	vm.RunTests("TestLVMPackage|TestLVMExists")

	return nil
}
