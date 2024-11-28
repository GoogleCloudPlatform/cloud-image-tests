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
	Name         = "acceleratorrdma"
	a3uNode1Name = "a3ultraNode1"
	a3uNode2Name = "a3ultraNode2"
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

	a3UltraNicConfig := []*computeBeta.NetworkInterface{
		{
			NicType:    "GVNIC",
			Network:    fmt.Sprintf("projects/%s/global/networks/%s", t.Project.Name, "a3ultra-test-net-0-do-not-delete"),
			Subnetwork: fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", t.Project.Name, testRegion, "a3ultra-test-sub-0-do-not-delete"),
		},
		{
			NicType:    "GVNIC",
			Network:    fmt.Sprintf("projects/%s/global/networks/%s", "compute-image-test-pool-001", "a3ultra-test-net-1-do-not-delete"),
			Subnetwork: fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", "compute-image-test-pool-001", testRegion, "a3ultra-test-sub-1-do-not-delete"),
		},
		// go/go-style/decisions#nil-slices
		// "Do not create APIs that force their clients to make distinctions
		// between nil and the empty slice."
		//
		// This is bad readability-wise, but we are using an API that makes
		// distinctions between nil and empty slices so not much choice.
		{
			NicType:       "MRDMA",
			Network:       fmt.Sprintf("projects/%s/global/networks/%s", t.Project.Name, "a3ultra-test-mrdma-do-not-delete"),
			Subnetwork:    fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", t.Project.Name, testRegion, "a3ultra-test-mrdma-sub-0-do-not-delete"),
			AccessConfigs: []*computeBeta.AccessConfig{},
		},
		{
			NicType:       "MRDMA",
			Network:       fmt.Sprintf("projects/%s/global/networks/%s", t.Project.Name, "a3ultra-test-mrdma-do-not-delete"),
			Subnetwork:    fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", t.Project.Name, testRegion, "a3ultra-test-mrdma-sub-1-do-not-delete"),
			AccessConfigs: []*computeBeta.AccessConfig{},
		},
		{
			NicType:       "MRDMA",
			Network:       fmt.Sprintf("projects/%s/global/networks/%s", t.Project.Name, "a3ultra-test-mrdma-do-not-delete"),
			Subnetwork:    fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", t.Project.Name, testRegion, "a3ultra-test-mrdma-sub-2-do-not-delete"),
			AccessConfigs: []*computeBeta.AccessConfig{},
		},
		{
			NicType:       "MRDMA",
			Network:       fmt.Sprintf("projects/%s/global/networks/%s", t.Project.Name, "a3ultra-test-mrdma-do-not-delete"),
			Subnetwork:    fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", t.Project.Name, testRegion, "a3ultra-test-mrdma-sub-3-do-not-delete"),
			AccessConfigs: []*computeBeta.AccessConfig{},
		},
		{
			NicType:       "MRDMA",
			Network:       fmt.Sprintf("projects/%s/global/networks/%s", t.Project.Name, "a3ultra-test-mrdma-do-not-delete"),
			Subnetwork:    fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", t.Project.Name, testRegion, "a3ultra-test-mrdma-sub-4-do-not-delete"),
			AccessConfigs: []*computeBeta.AccessConfig{},
		},
		{
			NicType:       "MRDMA",
			Network:       fmt.Sprintf("projects/%s/global/networks/%s", t.Project.Name, "a3ultra-test-mrdma-do-not-delete"),
			Subnetwork:    fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", t.Project.Name, testRegion, "a3ultra-test-mrdma-sub-5-do-not-delete"),
			AccessConfigs: []*computeBeta.AccessConfig{},
		},
		{
			NicType:       "MRDMA",
			Network:       fmt.Sprintf("projects/%s/global/networks/%s", t.Project.Name, "a3ultra-test-mrdma-do-not-delete"),
			Subnetwork:    fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", t.Project.Name, testRegion, "a3ultra-test-mrdma-sub-6-do-not-delete"),
			AccessConfigs: []*computeBeta.AccessConfig{},
		},
		{
			NicType:       "MRDMA",
			Network:       fmt.Sprintf("projects/%s/global/networks/%s", t.Project.Name, "a3ultra-test-mrdma-do-not-delete"),
			Subnetwork:    fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", t.Project.Name, testRegion, "a3ultra-test-mrdma-sub-7-do-not-delete"),
			AccessConfigs: []*computeBeta.AccessConfig{},
		},
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
