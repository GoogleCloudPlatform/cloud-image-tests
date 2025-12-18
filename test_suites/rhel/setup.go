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

// Package rhel is a CIT suite for validating the RHEL image.
package rhel

import (
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "rhel"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	image := t.Image
	if !utils.IsRHEL(image.Name) {
		t.Skip("Skipping RHEL test for non-RHEL images.")
	}

	vm, err := t.CreateTestVM("rhel")
	if err != nil {
		return err
	}

	rhelVersion := strings.TrimPrefix(regexp.MustCompile("rhel-[0-9]{1,2}-[0-9]{0,2}").FindString(image.Name), "rhel-")
	rhelVersionSplit := strings.Split(rhelVersion, "-")
	rhelMinorVersion := ""

	if len(rhelVersionSplit) > 1 {
		rhelMinorVersion = rhelVersionSplit[1]
	}

	vm.AddMetadata("rhel-major-version", rhelVersionSplit[0])
	vm.AddMetadata("rhel-minor-version", rhelMinorVersion)

	vm.RunTests("TestVersionLock")
	vm.RunTests("TestRhuiPackage")
	return nil
}
