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

// Package acceleratorrdma validates rdma stacks on accelerator images.
package acceleratorrdma

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/compute-daisy"
	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "acceleratorrdma"

// TestSetup sets up test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	t.LockProject()
	a3ultraVM := &daisy.InstanceBeta{}
	a3ultraVM.Name = "a3ultra_gpudirectrdma"
	a3ultraVM.MachineType = "a3-ultragpu-8g-nolssd"
	a3ultraVM.GuestAccelerators = []*computeBeta.AcceleratorConfig{
		{
			AcceleratorCount: 8,
			// This may need to be updated to the appropriate zone upon A3U release.
			AcceleratorType: "zones/us-east4-a/acceleratorTypes/nvidia-h200-141gb",
		},
	}
	acceleratorrdmaNetwork, err := t.CreateNetwork("acceleratorrdma", false)
	if err != nil {
		return err
	}
	gvnic0Subnet, err := acceleratorrdmaNetwork.CreateSubnetwork("rdma-gvnic-0", "10.1.2.0/24")
	if err != nil {
		return err
	}
	gvnic0Subnet.SetRegion("us-east4")
	gvnic1Subnet, err := acceleratorrdmaNetwork.CreateSubnetwork("rdma-gvnic-1", "10.1.3.0/24")
	if err != nil {
		return err
	}
	gvnic1Subnet.SetRegion("us-east4")
	mrdmaSubnet, err := acceleratorrdmaNetwork.CreateSubnetwork("rdma-mrdma", "10.1.4.0/24")
	if err != nil {
		return err
	}
	mrdmaSubnet.SetRegion("us-east4")
	if err := acceleratorrdmaNetwork.CreateFirewallRule("rdma-allow-tcp", "tcp", nil, []string{"10.1.2.0/24", "10.1.3.0/24", "10.1.4.0/24"}); err != nil {
		return err
	}
	if err := acceleratorrdmaNetwork.CreateFirewallRule("rdma-allow-udp", "udp", nil, []string{"10.1.2.0/24", "10.1.3.0/24", "10.1.4.0/24"}); err != nil {
		return err
	}
	if err := acceleratorrdmaNetwork.CreateFirewallRule("rdma-allow-icmp", "icmp", nil, []string{"10.1.2.0/24", "10.1.3.0/24", "10.1.4.0/24"}); err != nil {
		return err
	}
	a3ultraVM.NetworkInterfaces = []*computeBeta.NetworkInterface{
		{
			NicType:    "GVNIC",
			Subnetwork: "rdma-gvnic-0",
		},
		{
			NicType:    "GVNIC",
			Subnetwork: "rdma-gvnic-1",
		},
		{
			NicType:    "MRDMA",
			Subnetwork: "rdma-mrdma",
		},
		{
			NicType:    "MRDMA",
			Subnetwork: "rdma-mrdma",
		},
		{
			NicType:    "MRDMA",
			Subnetwork: "rdma-mrdma",
		},
		{
			NicType:    "MRDMA",
			Subnetwork: "rdma-mrdma",
		},
		{
			NicType:    "MRDMA",
			Subnetwork: "rdma-mrdma",
		},
		{
			NicType:    "MRDMA",
			Subnetwork: "rdma-mrdma",
		},
		{
			NicType:    "MRDMA",
			Subnetwork: "rdma-mrdma",
		},
		{
			NicType:    "MRDMA",
			Subnetwork: "rdma-mrdma",
		},
	}
	a3ultraVM.Scheduling = &computeBeta.Scheduling{OnHostMaintenance: "TERMINATE"}
	disks := []*compute.Disk{
		{Name: a3ultraVM.Name, Type: imagetest.HyperdiskBalanced, Zone: "us-east4-a"},
	}
	a3ultraVM.Zone = "us-east4-a"

	a3ultraTestVM, err := t.CreateTestVMFromInstanceBeta(a3ultraVM, disks)
	if err != nil {
		return err
	}
	a3ultraTestVM.RunTests("TestA3UltraGPUDirectRDMA")

	return nil
}
