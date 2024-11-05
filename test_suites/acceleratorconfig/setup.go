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

// Package acceleratorconfig contains tests for validating accelerator VM configuration.
package acceleratorconfig

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/compute-daisy"
	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "acceleratorconfig"

// TestSetup sets up test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	network1, err := t.CreateNetwork("a3u-net1", false)
	if err != nil {
		return err
	}
	subnetwork1, err := network1.CreateSubnetwork("subnetwork-1", "10.128.0.0/24")
	if err != nil {
		return err
	}
	subnetwork1.SetRegion("us-east4")

	subnetwork2, err := network1.CreateSubnetwork("subnetwork-2", "10.128.1.0/24")
	if err != nil {
		return err
	}
	subnetwork2.SetRegion("us-east4")

	subnetwork3, err := network1.CreateSubnetwork("subnetwork-3", "10.128.2.0/24")
	if err != nil {
		return err
	}
	subnetwork3.SetRegion("us-east4")

	vm := &daisy.InstanceBeta{}
	vm.Name = "gpucount"
	vm.MachineType = "a3-ultragpu-8g-nolssd"
	vm.NetworkInterfaces = []*computeBeta.NetworkInterface{
		{
			NicType:    "GVNIC",
			Network:    "global/networks/a3u-net1",
			Subnetwork: "regions/us-east4/subnetworks/subnetwork-1",
		},
		{
			NicType:    "GVNIC",
			Network:    "global/networks/a3u-net1",
			Subnetwork: "regions/us-east4/subnetworks/subnetwork-2",
		},
		{
			NicType:    "MRDMA",
			Network:    "global/networks/a3u-net1",
			Subnetwork: "regions/us-east4/subnetworks/subnetwork-3",
		},
		{
			NicType:    "MRDMA",
			Network:    "global/networks/a3u-net1",
			Subnetwork: "regions/us-east4/subnetworks/subnetwork-3",
		},
		{
			NicType:    "MRDMA",
			Network:    "global/networks/a3u-net1",
			Subnetwork: "regions/us-east4/subnetworks/subnetwork-3",
		},
		{
			NicType:    "MRDMA",
			Network:    "global/networks/a3u-net1",
			Subnetwork: "regions/us-east4/subnetworks/subnetwork-3",
		},
		{
			NicType:    "MRDMA",
			Network:    "global/networks/a3u-net1",
			Subnetwork: "regions/us-east4/subnetworks/subnetwork-3",
		},
		{
			NicType:    "MRDMA",
			Network:    "global/networks/a3u-net1",
			Subnetwork: "regions/us-east4/subnetworks/subnetwork-3",
		},
		{
			NicType:    "MRDMA",
			Network:    "global/networks/a3u-net1",
			Subnetwork: "regions/us-east4/subnetworks/subnetwork-3",
		},
		{
			NicType:    "MRDMA",
			Network:    "global/networks/a3u-net1",
			Subnetwork: "regions/us-east4/subnetworks/subnetwork-3",
		},
	}
	vm.GuestAccelerators = []*computeBeta.AcceleratorConfig{
		{
			AcceleratorCount: 8,
			// This may need to be updated to the appropriate zone upon A3U release.
			AcceleratorType: "zones/us-east4-a/acceleratorTypes/nvidia-h200-141gb",
		},
	}
	vm.Scheduling = &computeBeta.Scheduling{OnHostMaintenance: "TERMINATE"}
	disks := []*compute.Disk{
		{Name: vm.Name, Type: imagetest.PdBalanced, Zone: "us-east4-a"},
	}
	vm.Zone = "us-east4-a"

	tvm, err := t.CreateTestVMFromInstanceBeta(vm, disks)
	if err != nil {
		return err
	}
	tvm.RunTests("TestA3UGpuCount|TestA3UNicCount|TestA3UGPUNumaMapping|TestA3UNICNumaMapping")
	return nil
}
