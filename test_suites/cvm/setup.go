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

// Package cvm is a CIT suite for testing confidential computing features.
package cvm

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "cvm"

// TestSetup sets up test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	for _, feature := range t.Image.GuestOsFeatures {
		switch feature.Type {
		case "SEV_CAPABLE":
			sevtests := "TestSEVEnabled"
			vm := &daisy.InstanceBeta{}
			vm.Name = "sev"
			vm.ConfidentialInstanceConfig = &computeBeta.ConfidentialInstanceConfig{
				ConfidentialInstanceType:  "SEV",
				EnableConfidentialCompute: true,
			}
			if utils.HasFeature(t.Image, "SEV_LIVE_MIGRATABLE_V2") {
				sevtests += "|TestLiveMigrate"
				vm.Scopes = append(vm.Scopes, "https://www.googleapis.com/auth/cloud-platform")
				vm.Scheduling = &computeBeta.Scheduling{OnHostMaintenance: "MIGRATE"}
			} else {
				vm.Scheduling = &computeBeta.Scheduling{OnHostMaintenance: "TERMINATE"}
			}
			vm.MachineType = "n2d-standard-2"
			vm.MinCpuPlatform = "AMD Milan"
			disks := []*compute.Disk{{Name: vm.Name, Type: imagetest.PdBalanced}}
			tvm, err := t.CreateTestVMFromInstanceBeta(vm, disks)
			if err != nil {
				return err
			}
			tvm.RunTests(sevtests)
		case "SEV_SNP_CAPABLE":
			vm := &daisy.InstanceBeta{}
			vm.Name = "sevsnp"
			vm.Zone = "us-central1-a" // SEV_SNP not available in all regions
			vm.ConfidentialInstanceConfig = &computeBeta.ConfidentialInstanceConfig{
				ConfidentialInstanceType:  "SEV_SNP",
				EnableConfidentialCompute: true,
			}
			vm.Scheduling = &computeBeta.Scheduling{OnHostMaintenance: "TERMINATE"}
			vm.MachineType = "n2d-standard-2"
			vm.MinCpuPlatform = "AMD Milan"
			disks := []*compute.Disk{
				{Name: vm.Name, Type: imagetest.PdBalanced, Zone: "us-central1-a"},
			}
			tvm, err := t.CreateTestVMFromInstanceBeta(vm, disks)
			if err != nil {
				return err
			}
			tvm.RunTests("TestSEVSNPEnabled")
		case "TDX_CAPABLE":
			vm := &daisy.InstanceBeta{}
			vm.Name = "tdx"
			vm.Zone = "us-central1-a" // TDX not available in all regions
			vm.ConfidentialInstanceConfig = &computeBeta.ConfidentialInstanceConfig{
				ConfidentialInstanceType:  "TDX",
				EnableConfidentialCompute: true,
			}
			vm.Scheduling = &computeBeta.Scheduling{OnHostMaintenance: "TERMINATE"}
			vm.MachineType = "c3-standard-2"
			vm.MinCpuPlatform = "Intel Sapphire Rapids"
			disks := []*compute.Disk{
				{Name: vm.Name, Type: imagetest.PdBalanced, Zone: "us-central1-a"},
			}
			tvm, err := t.CreateTestVMFromInstanceBeta(vm, disks)
			if err != nil {
				return err
			}
			tvm.RunTests("TestTDXEnabled")
		}
	}
	return nil
}
