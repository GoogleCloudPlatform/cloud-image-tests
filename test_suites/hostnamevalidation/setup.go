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
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "hostnamevalidation"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	vm1, err := t.CreateTestVM("vm1")
	if err != nil {
		return err
	}
	vm1.RunTests("TestHostname|TestFQDN|TestHostKeysGeneratedOnce|TestHostsFile")
	// custom host name test not yet implemented for windows
	if !utils.HasFeature(t.Image, "WINDOWS") {
		vm2, err := t.CreateTestVM("vm2.custom.domain")
		if err != nil {
			return err
		}
		vm2.RunTests("TestCustomHostname")
	}

	return nil
}
