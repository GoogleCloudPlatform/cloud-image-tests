// Copyright 2026 Google LLC.
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

// Package networkconfig is a CIT suite for testing network configuration functionality.
package networkconfig

import (
	"fmt"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
const Name = "networkconfig"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	vm, err := createMachnie(t, t.MachineType.Name, t.Zone.Name)
	if err != nil {
		return err
	}
	vm.RunTests("TestNICNames|TestDeviceConfig")

	return nil
}

func createMachnie(t *imagetest.TestWorkflow, machineType string, zone string) (*imagetest.TestVM, error) {
	disk := compute.Disk{
		Name: "machine",
		Type: imagetest.DiskTypeNeeded(machineType),
		Zone: zone,
	}

	instance := &daisy.Instance{}

	vm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{&disk}, instance)
	if err != nil {
		return nil, fmt.Errorf("creating machine: %w", err)
	}
	vm.ForceMachineType(machineType)
	vm.ForceZone(zone)

	return vm, nil
}
