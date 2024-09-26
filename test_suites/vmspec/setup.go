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

// Package vmspec is a test suite that tests that things work after vmspec
// changes.
package vmspec

import (
	"fmt"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "vmspec"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	// Skip ARM64 images, since no ARM64-supporting machine types support LSSDs.
	if t.Image.Architecture == "ARM64" {
		t.Skip("vmspec not supported on ARM images")
		return nil
	}

	// Create new networks and subnetworks for multinic.
	network1, err := t.CreateNetwork("test-network", false)
	if err != nil {
		return fmt.Errorf("failed to create test network: %v", err)
	}
	subnet1, err := network1.CreateSubnetwork("test-subnetwork-1", "10.128.0.0/16")
	if err != nil {
		return fmt.Errorf("failed to create test subnetwork 1: %v", err)
	}
	subnet1.SetRegion("us-central1")

	network2, err := t.CreateNetwork("test-network-2", false)
	if err != nil {
		return fmt.Errorf("failed to create test network 2: %v", err)
	}
	subnet2, err := network2.CreateSubnetwork("test-subnetwork-2", "10.0.0.0/24")
	if err != nil {
		return fmt.Errorf("failed to create test subnetwork 2: %v", err)
	}
	subnet2.SetRegion("us-central1")

	// Create the source VM. The VMs will be made and run in us-central1-a.
	zone, err := t.Client.GetZone(t.Project.Name, "us-central1-a")
	if err != nil {
		return fmt.Errorf("failed to get zone: %v", err)
	}
	sourceInst := &daisy.Instance{}
	disks := []*compute.Disk{&compute.Disk{Name: "source", Type: imagetest.PdBalanced, Zone: zone.Name}}
	source, err := t.CreateTestVMMultipleDisks(disks, sourceInst)
	if err != nil {
		return err
	}
	source.ForceMachineType("c3-standard-4")
	source.ForceZone("us-central1-a")
	source.RunTests("TestEmpty")

	// Create a derivative VM. This is the actual meat of the test.
	vmspec, err := source.CreateDerivativeVM("lssd")
	if err != nil {
		return err
	}

	// The machine type should stay in the same generation as the source VM.
	vmspec.ForceMachineType("c3-standard-8-lssd")
	vmspec.ForceZone("us-central1-a")
	if err := vmspec.AddCustomNetwork(network1, subnet1); err != nil {
		return err
	}
	if err := vmspec.AddCustomNetwork(network2, subnet2); err != nil {
		return err
	}

	pingTest := "TestPing"
	if utils.HasFeature(t.Image, "WINDOWS") {
		pingTest = "TestWindowsPing"
	}
	vmspec.RunTests(fmt.Sprintf("TestPCIEChanged|%s|TestMetadataServer", pingTest))
	if err := t.WaitForVMQuota(&daisy.QuotaAvailable{Metric: "C3_CPUS", Units: 8, Region: "us-central1"}); err != nil {
		return err
	}

	return nil
}
