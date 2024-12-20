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
	"fmt"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/compute-daisy"
	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
)

var (
	// Name is the name of the test package. It must match the directory name.
	Name              = "acceleratorrdma"
	a3uNode1Name      = "a3ultraNode1"
	a3uNode2Name      = "a3ultraNode2"
	testRegion        = "europe-west1"
	testZone          = "europe-west1-b"
	gvnicNet0Name     = "gvnic-net0"
	gvnicNet0Sub0Name = "gvnic-net0-sub0"
	gvnicNet1Name     = "gvnic-net1"
	gvnicNet1Sub0Name = "gvnic-net1-sub0"
	mrdmaNetName      = "mrdma-net"
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

	gvnicNet0, err := t.CreateNetwork(gvnicNet0Name, false)
	if err != nil {
		return err
	}
	_, err = gvnicNet0.CreateSubnetwork(gvnicNet0Sub0Name, "192.168.0.0/24")
	if err != nil {
		return err
	}

	gvnicNet1, err := t.CreateNetwork(gvnicNet1Name, false)
	if err != nil {
		return err
	}
	_, err = gvnicNet1.CreateSubnetwork(gvnicNet1Sub0Name, "192.168.1.0/24")
	if err != nil {
		return err
	}

	mrdma := &daisy.Network{}
	mrdma.Name = mrdmaNetName
	mrdma.Mtu = 8896                        // Max allowed value
	mrdma.AutoCreateSubnetworks = new(bool) // false
	mrdma.NetworkProfile = fmt.Sprintf("global/networkProfiles/%s-vpc-roce", testZone)
	mrdmaNet, err := t.CreateNetworkFromDaisyNetwork(mrdma)
	if err != nil {
		return err
	}

	a3UltraNicConfig := []*computeBeta.NetworkInterface{
		{
			NicType:    "GVNIC",
			Network:    gvnicNet0Name,
			Subnetwork: gvnicNet0Sub0Name,
		},
		{
			NicType:    "GVNIC",
			Network:    gvnicNet1Name,
			Subnetwork: gvnicNet1Sub0Name,
		},
	}
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("mrdma-net-sub-%d", i)
		_, err := mrdmaNet.CreateSubnetwork(name, fmt.Sprintf("192.168.%d.0/24", i+2))
		if err != nil {
			return err
		}
		// go/go-style/decisions#nil-slices
		// "Do not create APIs that force their clients to make distinctions
		// between nil and the empty slice."
		//
		// This is bad readability-wise, but we are using an API that makes
		// distinctions between nil and empty slices so not much choice.
		a3UltraNicConfig = append(a3UltraNicConfig, &computeBeta.NetworkInterface{
			NicType:       "MRDMA",
			Network:       mrdmaNetName,
			Subnetwork:    name,
			AccessConfigs: []*computeBeta.AccessConfig{},
		})
	}
	a3ultraSchedulingConfig := &computeBeta.Scheduling{OnHostMaintenance: "TERMINATE"}

	a3UltraNode1 := &daisy.InstanceBeta{}
	a3UltraNode1.Name = a3uNode1Name
	a3UltraNode1.MachineType = "a3-ultragpu-8g"
	a3UltraNode1.Zone = testZone
	a3UltraNode1.Scheduling = a3ultraSchedulingConfig
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
	a3UltraNode2.MachineType = "a3-ultragpu-8g"
	a3UltraNode2.Zone = testZone
	a3UltraNode2.Scheduling = a3ultraSchedulingConfig
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
