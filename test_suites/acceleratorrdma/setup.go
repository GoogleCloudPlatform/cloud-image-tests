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

var (
	// Name is the name of the test package. It must match the directory name.
	Name         = "acceleratorrdma"
	a3uNode1Name = "a3ultra-node1"
	a3uNode2Name = "a3ultra-node2"
	testRegion   = "europe-west1"
	testZone     = "europe-west1-b"
)

// TestSetup sets up test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	t.LockProject()
	a3UltraAccelConfig := []*computeBeta.AcceleratorConfig{
		{
			AcceleratorCount: 8,
			AcceleratorType:  "zones/" + testZone + "/acceleratorTypes/nvidia-h200-141gb",
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
	gvnic0Subnet.SetRegion(testRegion)
	gvnic1Subnet, err := acceleratorrdmaNetwork.CreateSubnetwork("rdma-gvnic-1", "10.1.3.0/24")
	if err != nil {
		return err
	}
	gvnic1Subnet.SetRegion(testRegion)
	mrdmaSubnet, err := acceleratorrdmaNetwork.CreateSubnetwork("rdma-mrdma", "10.1.4.0/24")
	if err != nil {
		return err
	}
	mrdmaSubnet.SetRegion(testRegion)
	if err := acceleratorrdmaNetwork.CreateFirewallRule("rdma-allow-tcp", "tcp", nil, []string{"10.1.2.0/24", "10.1.3.0/24", "10.1.4.0/24"}); err != nil {
		return err
	}
	if err := acceleratorrdmaNetwork.CreateFirewallRule("rdma-allow-udp", "udp", nil, []string{"10.1.2.0/24", "10.1.3.0/24", "10.1.4.0/24"}); err != nil {
		return err
	}
	if err := acceleratorrdmaNetwork.CreateFirewallRule("rdma-allow-icmp", "icmp", nil, []string{"10.1.2.0/24", "10.1.3.0/24", "10.1.4.0/24"}); err != nil {
		return err
	}
	a3UltraNicConfig := []*computeBeta.NetworkInterface{
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
	a3ultraSchedulingConfig := &computeBeta.Scheduling{OnHostMaintenance: "TERMINATE"}

	a3UltraNode1 := &daisy.InstanceBeta{}
	a3UltraNode1.Name = a3uNode1Name
	a3UltraNode1.MachineType = "a3-ultragpu-8g-nolssd"
	a3UltraNode1.Zone = testZone
	a3UltraNode1.Scheduling = a3ultraSchedulingConfig
	a3UltraNode1.Metadata = map[string]string{imagetest.ShouldRebootDuringTest: "true"}
	a3UltraNode1.NetworkInterfaces = a3UltraNicConfig
	a3UltraNode1.GuestAccelerators = a3UltraAccelConfig
	node1Disks := []*compute.Disk{{Name: a3uNode1Name, Type: imagetest.HyperdiskBalanced, Zone: testZone, SizeGb: 80}}

	a3UltraNode1VM, err := t.CreateTestVMFromInstanceBeta(a3UltraNode1, node1Disks)
	if err != nil {
		return err
	}
	a3UltraNode1VM.RunTests("TestA3UltraGPUDirectRDMAHost")

	a3UltraNode2 := &daisy.InstanceBeta{}
	a3UltraNode2.Name = a3uNode2Name
	a3UltraNode2.MachineType = "a3-ultragpu-8g-nolssd"
	a3UltraNode2.Zone = testZone
	a3UltraNode2.Scheduling = a3ultraSchedulingConfig
	a3UltraNode2.Metadata = map[string]string{imagetest.ShouldRebootDuringTest: "true"}
	a3UltraNode2.NetworkInterfaces = a3UltraNicConfig
	a3UltraNode2.GuestAccelerators = a3UltraAccelConfig
	node2Disks := []*compute.Disk{{Name: a3uNode2Name, Type: imagetest.HyperdiskBalanced, Zone: testZone, SizeGb: 80}}

	a3UltraNode2VM, err := t.CreateTestVMFromInstanceBeta(a3UltraNode2, node2Disks)
	if err != nil {
		return err
	}
	a3UltraNode2VM.RunTests("TestA3UltraGPUDirectRDMAClient")
	return nil
}
