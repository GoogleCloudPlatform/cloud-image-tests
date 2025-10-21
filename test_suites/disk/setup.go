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

// Package disk is a CIT suite for testing basic disk functionality.
package disk

import (
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

type blockdevNamingConfig struct {
	machineType string
	arch        string
}

var (
	// Name is the name of the test package. It must match the directory name.
	Name                = "disk"
	blockdevNamingCases = []blockdevNamingConfig{
		{
			machineType: "c4a-standard-1",
			arch:        "ARM64",
		},
		{
			machineType: "c3-standard-4",
			arch:        "X86_64",
		},
	}
)

const (
	resizeDiskSize = 200
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	rebootInst := &daisy.Instance{}
	rebootInst.Metadata = map[string]string{imagetest.ShouldRebootDuringTest: "true"}
	vm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "resize"}}, rebootInst)
	if err != nil {
		return err
	}
	// TODO:currently the Resize and Reboot disk test is only written to run on linux
	if !utils.HasFeature(t.Image, "WINDOWS") {
		if err = vm.ResizeDiskAndReboot(resizeDiskSize); err != nil {
			return err
		}
	}
	vm.RunTests("TestDiskReadWrite|TestDiskResize")
	// Block device naming is an interaction between OS and hardware alone on windows, there is no guest-environment equivalent of udev rules for us to test.
	if !utils.HasFeature(t.Image, "WINDOWS") && utils.HasFeature(t.Image, "GVNIC") {
		for _, tc := range blockdevNamingCases {
			if tc.arch != t.Image.Architecture {
				continue
			}
			inst := &daisy.Instance{}
			inst.MachineType = tc.machineType
			series, _, _ := strings.Cut(tc.machineType, "-")
			inst.Name = "blockNaming" + strings.ToUpper(series)
			vm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: inst.Name, Type: imagetest.HyperdiskBalanced}, {Name: "secondary", Type: imagetest.HyperdiskBalanced, SizeGb: 10}}, inst)
			if err != nil {
				return err
			}
			vm.RunTests("TestBlockDeviceNaming")
		}
	}
	return nil
}
