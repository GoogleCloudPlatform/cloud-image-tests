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
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
)

// Name is the name of the test package. It must match the directory name.
var Name = "ssh"

const (
	user  = "test-user"
	user2 = "test-user2"
	user3 = "test-user3"
	user4 = "test-user4"
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	// adds the private key to the t.wf.Sources
	publicKey, err := t.AddSSHKey(user)
	if err != nil {
		return err
	}
	publicKey2, err := t.AddSSHKey(user2)
	if err != nil {
		return err
	}
	publicKey3, err := t.AddSSHKey(user3)
	if err != nil {
		return err
	}
	// This public key is passed down to the server instance through metadata,
	// this key is then used to change the user's key in the TestSSHChangeKey.
	publicKey4, err := t.AddSSHKey(user4)
	if err != nil {
		return err
	}
	vm, err := t.CreateTestVM("client")
	if err != nil {
		return err
	}
	vm.AddScope("https://www.googleapis.com/auth/cloud-platform")
	vm.AddMetadata("block-project-ssh-keys", "true")
	vm.AddMetadata("enable-guest-attributes", "true")
	vm.AddMetadata("enable-windows-ssh", "true")
	vm.AddMetadata("sysprep-specialize-script-cmd", "googet -noconfirm=true install google-compute-engine-ssh")
	runTests := "TestSSHInstanceKey|TestHostKeysAreUnique|TestMatchingKeysInGuestAttributes|TestDeleteUserDefault"

	if !strings.Contains(t.Image.Name, "windows") {
		vm4, err := t.CreateTestVM("server2")
		if err != nil {
			return err
		}
		vm4.AddUser(user, publicKey)
		vm4.AddUser(user2, publicKey2)
		vm4.AddUser(user3, publicKey3)

		vm4.AddMetadata("target-public-key", publicKey4)

		vm4.AddScope("https://www.googleapis.com/auth/cloud-platform")
		vm4.AddMetadata("enable-guest-attributes", "true")
		vm4.AddMetadata("enable-oslogin", "false")
		vm4.RunTests("TestSSHChangeKey|TestSwitchDefaultConfig")
		runTests += "|TestDeleteLocalUser"
	}
	vm.RunTests(runTests)

	vm2, err := t.CreateTestVM("server")
	if err != nil {
		return err
	}
	vm2.AddUser(user, publicKey)
	vm2.AddUser(user2, publicKey2)

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
