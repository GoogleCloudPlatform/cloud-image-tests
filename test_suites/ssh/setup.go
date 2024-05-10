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

// Package ssh tests guest agent metadata ssh key setup.
package ssh

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
)

// Name is the name of the test package. It must match the directory name.
var Name = "ssh"

const user = "test-user"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	// adds the private key to the t.wf.Sources
	publicKey, err := t.AddSSHKey(user)
	if err != nil {
		return err
	}
	vm, err := t.CreateTestVM("client")
	if err != nil {
		return err
	}
	vm.AddMetadata("block-project-ssh-keys", "true")
	vm.AddMetadata("enable-guest-attributes", "true")
	vm.AddMetadata("enable-windows-ssh", "true")
	vm.AddMetadata("sysprep-specialize-script-cmd", "googet -noconfirm=true install google-compute-engine-ssh")
	vm.RunTests("TestSSHInstanceKey|TestHostKeysAreUnique|TestMatchingKeysInGuestAttributes")

	vm2, err := t.CreateTestVM("server")
	if err != nil {
		return err
	}
	vm2.AddUser(user, publicKey)
	vm2.AddMetadata("enable-guest-attributes", "true")
	vm2.AddMetadata("enable-oslogin", "false")
	vm2.AddMetadata("enable-windows-ssh", "true")
	vm2.AddMetadata("sysprep-specialize-script-cmd", "googet -noconfirm=true install google-compute-engine-ssh")
	vm2.RunTests("TestEmptyTest")

	vm3, err := t.CreateTestVM("hostkeysafteragentrestart")
	if err != nil {
		return err
	}
	vm3.RunTests("TestHostKeysNotOverrideAfterAgentRestart")
	return nil
}
