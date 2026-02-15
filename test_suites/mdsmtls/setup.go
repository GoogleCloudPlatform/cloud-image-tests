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

// Package mdsmtls is a CIT suite for testing mtls communication with the mds.
package mdsmtls

import (
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// TODO(tylerdao): Comment testing CIT Copybara, remove this line once the config has been fixed

// Name is the name of the test package. It must match the directory name.
const Name = "mdsmtls"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if !utils.HasFeature(t.Image, "UEFI_COMPATIBLE") {
		return nil
	}

	// TODO(b/479320912): Remove this exception once the bug is fixed.
	updatedAgent := strings.Contains(t.Image.Name, "guest-agent") && !strings.Contains(t.Image.Name, "guest-agent-stable")

	vm, err := t.CreateTestVM("mtlscreds")
	if err != nil {
		return err
	}
	if !updatedAgent {
		// Old agents have it disabled by default.
		vm.AddMetadata("disable-https-mds-setup", "FALSE")
	}

	vm2, err := t.CreateTestVM("mtlscredsdisabled")
	if err != nil {
		return err
	}
	if updatedAgent {
		// New agents have it enabled by default, so we need to explicitly disable it.
		vm2.AddMetadata("disable-https-mds-setup", "TRUE")
	}

	vm3, err := t.CreateTestVM("mtlscredswithtruststore")
	if err != nil {
		return err
	}
	if !updatedAgent {
		// Old agents have it disabled by default.
		vm3.AddMetadata("disable-https-mds-setup", "FALSE")
	}
	vm3.AddMetadata("enable-https-mds-native-cert-store", "TRUE")

	vm.RunTests("TestMTLSCredsExists|TestMTLSJobScheduled")
	vm2.RunTests("TestDisabled")
	vm3.RunTests("TestCredsExistsInOSTrustStore")
	return nil
}
