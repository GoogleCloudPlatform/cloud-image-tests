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

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/compute-daisy"
	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var (
	Name       = "acceleratorconfig"
	testRegion = "europe-west1"
	testZone   = "europe-west1-b"
)

// TestSetup sets up test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	vm := &daisy.InstanceBeta{}
	vm.Name = "a3ultraconfig"
	vm.MachineType = "a3-ultragpu-8g-nolssd"
	vm.NetworkInterfaces = []*computeBeta.NetworkInterface{
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
	vm.GuestAccelerators = []*computeBeta.AcceleratorConfig{
		{
			AcceleratorCount: 8,
			// This may need to be updated to the appropriate zone upon A3U release.
			AcceleratorType: "zones/" + testZone + "/acceleratorTypes/nvidia-h200-141gb",
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
	tvm.RunTests("TestA3UGpuCount|TestA3UNicCount|TestA3UGPUNumaMapping|TestA3UNICNumaMapping")
	return nil
}
