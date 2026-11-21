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

// Package pluginmanager contains the test suite for testing the Plugin Manager
// related features.
package pluginmanager

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "pluginmanager"

const (
	// vmExtLabelKey is the instance label the lifecycle test's policy selects
	// on; vmExtLabelMetadataKey tells the guest the value it was given.
	vmExtLabelKey         = "cit-vmext"
	vmExtLabelMetadataKey = "cit-vmext-label"
	vmExtExtension        = "ops-agent"
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	defaultVM, err := t.CreateTestVM("plugincleanup")
	if err != nil {
		return err
	}
	defaultVM.RunTests("TestPluginCleanup")

	// SLES and Ubuntu images ship the guest agent without the core plugin.
	// Images built by the guest-agent pipeline install it.
	if !strings.Contains(t.Image.Name, "guest-agent") && (strings.Contains(t.Image.Name, "sles") || strings.Contains(t.Image.Name, "ubuntu")) {
		noCorePluginVM, err := t.CreateTestVM("nocoreplugin")
		if err != nil {
			return err
		}
		noCorePluginVM.RunTests("TestPluginManagerNoCorePlugin")
	} else {
		localPluginVM, err := t.CreateTestVM("localplugin")
		if err != nil {
			return err
		}
		localPluginVM.RunTests("TestLocalPlugin")
		pluginStopVM, err := t.CreateTestVM("pluginstop")
		if err != nil {
			return err
		}
		pluginStopVM.RunTests("TestCorePluginStop")
		disableACSVM, err := t.CreateTestVM("acsdisabled")
		if err != nil {
			return err
		}
		disableACSVM.RunTests("TestACSDisabled")
	}

	// The Ops Agent does not support COS.
	if !utils.HasFeature(t.Image, "WINDOWS") && !utils.IsCOS(t.Image.Name) {
		// A label only this VM carries, so policies from other runs in the zone
		// never select it.
		label := fmt.Sprintf("%x", rand.Uint64())
		inst := &daisy.Instance{}
		inst.Labels = map[string]string{vmExtLabelKey: label}
		inst.Metadata = map[string]string{
			imagetest.ShouldRebootDuringTest: "true",
			vmExtLabelMetadataKey:            label,
		}
		lifecycleVM, err := t.CreateTestVMFromInstance(inst, []*compute.Disk{{Name: "vmextlifecycle"}})
		if err != nil {
			return err
		}
		lifecycleVM.AddScope("https://www.googleapis.com/auth/cloud-platform")
		lifecycleVM.RunTests("TestVMExtensionLifecycle")
		if err := lifecycleVM.Reboot(); err != nil {
			return err
		}
	}
	return nil
}
