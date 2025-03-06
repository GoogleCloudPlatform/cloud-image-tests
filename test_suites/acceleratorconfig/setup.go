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
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/compute-daisy"
	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var (
	Name              = "acceleratorconfig"
	gvnicNet0Name     = "gvnic-net0"
	gvnicNet0Sub0Name = "gvnic-net0-sub0"
	gvnicNet1Name     = "gvnic-net1"
	gvnicNet1Sub0Name = "gvnic-net1-sub0"
	mrdmaNetName      = "mrdma-net"
)

// TestSetup sets up test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	t.LockProject()

	testZone := t.Zone.Name
	// For example, region should be us-central1 for zone us-central1-a.
	lastDashIndex := strings.LastIndex(testZone, "-")
	if lastDashIndex == -1 {
		return fmt.Errorf("invalid zone: %s", testZone)
	}
	testRegion := testZone[:lastDashIndex]
	gvnicNet0, err := t.CreateNetwork(gvnicNet0Name, false)
	if err != nil {
		return err
	}
	gvnicNet0Sub0, err := gvnicNet0.CreateSubnetwork(gvnicNet0Sub0Name, "192.168.0.0/24")
	if err != nil {
		return err
	}
	gvnicNet0Sub0.SetRegion(testRegion)

	gvnicNet1, err := t.CreateNetwork(gvnicNet1Name, false)
	if err != nil {
		return err
	}
	gvnicNet1Sub0, err := gvnicNet1.CreateSubnetwork(gvnicNet1Sub0Name, "192.168.1.0/24")
	if err != nil {
		return err
	}
	gvnicNet1Sub0.SetRegion(testRegion)

	mrdma := &daisy.Network{}
	mrdma.Name = mrdmaNetName
	mrdma.Mtu = 8896                        // Max allowed value
	mrdma.AutoCreateSubnetworks = new(bool) // false
	mrdma.NetworkProfile = fmt.Sprintf("global/networkProfiles/%s-vpc-roce", testZone)
	mrdmaNet, err := t.CreateNetworkFromDaisyNetwork(mrdma)
	if err != nil {
		return err
	}

	vm := &daisy.InstanceBeta{}
	vm.Name = "accelerator-cfg"
	vm.MachineType = t.MachineType.Name
	vm.NetworkInterfaces = []*computeBeta.NetworkInterface{
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
		mrdmaSubnet, err := mrdmaNet.CreateSubnetwork(name, fmt.Sprintf("192.168.%d.0/24", i+2))
		if err != nil {
			return err
		}
		mrdmaSubnet.SetRegion(testRegion)
		// go/go-style/decisions#nil-slices
		// "Do not create APIs that force their clients to make distinctions
		// between nil and the empty slice."
		//
		// This is bad readability-wise, but we are using an API that makes
		// distinctions between nil and empty slices so not much choice.
		vm.NetworkInterfaces = append(vm.NetworkInterfaces, &computeBeta.NetworkInterface{
			NicType:       "MRDMA",
			Network:       mrdmaNetName,
			Subnetwork:    name,
			AccessConfigs: []*computeBeta.AccessConfig{},
		})
	}
	vm.GuestAccelerators = []*computeBeta.AcceleratorConfig{
		{
			AcceleratorCount: 8,
			AcceleratorType:  fmt.Sprintf("zones/%s/acceleratorTypes/%s", testZone, t.AcceleratorType),
		},
	}
	vm.Scheduling = &computeBeta.Scheduling{OnHostMaintenance: "TERMINATE"}
	disks := []*compute.Disk{
		{Name: vm.Name, Type: imagetest.HyperdiskBalanced, Zone: testZone},
	}
	vm.Zone = testZone

	tvm, err := t.CreateTestVMFromInstanceBeta(vm, disks)
	if err != nil {
		return err
	}
	tvm.RunTests("TestGpuCount|TestNicCount|TestGPUNumaMapping|TestNICNumaMapping|TestNICNaming")
	return nil
}
