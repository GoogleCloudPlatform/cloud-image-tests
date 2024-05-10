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

// Package packagevalidation tests that the guest environment and other
// necessary packages are installed and configured correctly.
package packagevalidation

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "packagevalidation"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	vm1, err := t.CreateTestVM("installedPackages")
	if err != nil {
		return err
	}
	vm1.RunTests("TestStandardPrograms|TestGuestPackages|TestNTP")

	// as part of the migration of the windows test suite, these vms
	// are only used to run windows tests. The tests themselves
	// have components which need to be run on different vms.
	if utils.HasFeature(t.Image, "WINDOWS") {
		vm2, err := t.CreateTestVM("googetFunctionality")
		if err != nil {
			return err
		}
		vm2.RunTests("TestGooGetInstalled|TestGooGetAvailable|TestSigned|TestRemoveInstall" +
			"|TestPackagesInstalled|TestPackagesAvailable|TestPackagesSigned")
		vm3, err := t.CreateTestVM("vm3")
		if err != nil {
			return err
		}
		vm3.RunTests("TestRepoManagement")
		vm4, err := t.CreateTestVM("repoManagement")
		if err != nil {
			return err
		}
		vm4.RunTests("TestNetworkDriverLoaded|TestDriversInstalled|TestDriversRemoved")
		// the former windows_image_validation test suite tests are run by this VM.
		// It may make sense to move some of these tests to other suites in the future.
		vm5, err := t.CreateTestVM("drivers")
		if err != nil {
			return err
		}
		vm5.RunTests("TestAutoUpdateEnabled|TestNetworkConnecton|TestEmsEnabled" +
			"|TestTimeZoneUTC|TestPowershellVersion|TestStartExe|TestDotNETVersion" +
			"|TestServicesState|TestWindowsEdition|TestWindowsCore|TestServerGuiShell")
		sysprepvm, err := t.CreateTestVM("gcesysprep")
		if err != nil {
			return err
		}
		sysprepvm.RunTests("TestGCESysprep")
	}
	return nil
}
