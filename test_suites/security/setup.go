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

// Package security tests security related OS settings are configured correctly.
package security

import "github.com/GoogleCloudPlatform/cloud-image-tests"

// Name is the name of the test package. It must match the directory name.
var Name = "security"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	vm, err := t.CreateTestVM("securitySetttings")
	vm.AddMetadata("enable-windows-ssh", "true")
	vm.AddMetadata("sysprep-specialize-script-cmd", "googet -noconfirm=true install google-compute-engine-ssh")
	return err
}
