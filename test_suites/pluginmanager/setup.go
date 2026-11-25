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
	"regexp"
	"strconv"
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
	// vmExtHybridMetadataKey tells the lifecycle VM whether the image runs the
	// legacy agent beside the plugin manager.
	vmExtHybridMetadataKey = "cit-vmext-hybrid"
	vmExtExtension         = "ops-agent"
)

// TestVMExtensionLifecycle runs where all of the following hold and skips
// with a reason otherwise:
//
//   - The extension has a build for the OS. ops-agent covers
//     https://cloud.google.com/logging/docs/agent/ops-agent#supported_operating_systems;
//     elsewhere the VM extension service reports INCOMPATIBLE and nothing
//     installs. vmExtUnsupportedImages lists the known gaps.
//   - The guest agent on the image has the plugin manager (ggactl_plugin).
//   - The VM service account may create VM extension policies and the project
//     has the extension's APIs enabled (test_suites/README.md).
var vmExtUnsupportedImages = []struct {
	reason string
	match  func(image string) bool
}{
	{"ops-agent has no COS build", utils.IsCOS},
	{"ops-agent has no Windows client build", utils.IsWindowsClient},
	{"ops-agent has no Oracle Linux build", utils.IsOracle},
	{"ops-agent has no builds for Ubuntu interim releases", isUbuntuInterim},
}

func vmExtUnsupportedReason(image string) string {
	for _, u := range vmExtUnsupportedImages {
		if u.match(image) {
			return u.reason
		}
	}
	return ""
}

// ubuntuRelease captures the YY and MM of an Ubuntu image name, for example
// ubuntu-2604-resolute or testing-ubuntu-guest-2404-noble.
var ubuntuRelease = regexp.MustCompile(`ubuntu\D*(\d\d)(\d\d)`)

// isUbuntuInterim reports Ubuntu releases other than the YY.04 LTS releases of
// even years.
func isUbuntuInterim(image string) bool {
	m := ubuntuRelease.FindStringSubmatch(image)
	if m == nil {
		return false
	}
	year, _ := strconv.Atoi(m[1])
	return m[2] != "04" || year%2 != 0
}

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	defaultVM, err := t.CreateTestVM("plugincleanup")
	if err != nil {
		return err
	}
	defaultVM.RunTests("TestPluginCleanup")

	// SLES and Ubuntu images ship the guest agent without the core plugin.
	// Images built by the guest-agent pipeline install it.
	hybrid := !strings.Contains(t.Image.Name, "guest-agent") && (strings.Contains(t.Image.Name, "sles") || strings.Contains(t.Image.Name, "ubuntu"))
	if hybrid {
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

	// A label only this VM carries, so policies from other runs in the zone
	// never select it.
	label := fmt.Sprintf("%x", rand.Uint64())
	inst := &daisy.Instance{}
	inst.Labels = map[string]string{vmExtLabelKey: label}
	inst.Metadata = map[string]string{
		imagetest.ShouldRebootDuringTest: "true",
		vmExtLabelMetadataKey:            label,
		vmExtHybridMetadataKey:           strconv.FormatBool(hybrid),
	}
	lifecycleVM, err := t.CreateTestVMFromInstance(inst, []*compute.Disk{{Name: "vmextlifecycle"}})
	if err != nil {
		return err
	}
	lifecycleVM.AddScope("https://www.googleapis.com/auth/cloud-platform")
	lifecycleVM.RunTests("TestVMExtensionLifecycle")
	return lifecycleVM.Reboot()
}
