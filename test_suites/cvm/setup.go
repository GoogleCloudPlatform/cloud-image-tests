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

// cvmZones lists the zones that support CVM machine types. Namely, SEV/SEV-SNP on N2D machines
// (AMD Milan CPU) and Intel TDX on C3 machines (Intel Sapphire Rapids CPU).
// See https://docs.cloud.google.com/confidential-computing/confidential-vm/docs/supported-configurations
var cvmZones = map[string]bool{
	"us-central1-a": true, "us-central1-b": true, "us-central1-c": true,
	"europe-west4-a": true, "europe-west4-b": true, "europe-west4-c": true,
	"asia-southeast1-a": true, "asia-southeast1-b": true, "asia-southeast1-c": true,
}

// TestSetup sets up test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	dedicatedZone := t.Zone.Name
	// Check if CLI zone is a valid CVM zone. If not, fall back to us-central1-a.
	if !cvmZones[dedicatedZone] {
		dedicatedZone = "us-central1-a"
	}
	for _, feature := range t.ImageBeta.GuestOsFeatures {
		switch feature.Type {
		case "SEV_CAPABLE":
			sevtests := "TestSEVEnabled|TestCheckApicId|TestCheckCpuidLeaf7"
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
			disks := []*compute.Disk{{Name: vm.Name, Type: imagetest.PdBalanced, Zone: dedicatedZone}}
			tvm, err := t.CreateTestVMFromInstanceBeta(vm, disks)
			if err != nil {
				return err
			}
			tvm.ForceZone(dedicatedZone)
			tvm.RunTests(sevtests)
		case "SEV_SNP_CAPABLE":
			sevsnptests := "TestSEVSNPEnabled|TestSEVSNPAttestation|TestCheckApicId|TestCheckCpuidLeaf7"
			vm := &daisy.InstanceBeta{}
			vm.Name = "sevsnp"
			vm.ConfidentialInstanceConfig = &computeBeta.ConfidentialInstanceConfig{
				ConfidentialInstanceType:  "SEV_SNP",
				EnableConfidentialCompute: true,
			}
			vm.Scheduling = &computeBeta.Scheduling{OnHostMaintenance: "TERMINATE"}
			vm.MachineType = "n2d-standard-2"
			vm.MinCpuPlatform = "AMD Milan"
			disks := []*compute.Disk{
				{Name: vm.Name, Type: imagetest.PdBalanced, Zone: dedicatedZone},
			}
			tvm, err := t.CreateTestVMFromInstanceBeta(vm, disks)
			if err != nil {
				return err
			}
			tvm.ForceZone(dedicatedZone)
			tvm.RunTests(sevsnptests)
		case "TDX_CAPABLE":
			tdxtests := "TestTDXEnabled|TestTDXAttestation|TestCheckApicId|TestCheckCpuidLeaf7"
			vm := &daisy.InstanceBeta{}
			vm.Name = "tdx"
			vm.ConfidentialInstanceConfig = &computeBeta.ConfidentialInstanceConfig{
				ConfidentialInstanceType:  "TDX",
				EnableConfidentialCompute: true,
			}
			vm.Scheduling = &computeBeta.Scheduling{OnHostMaintenance: "TERMINATE"}
			vm.MachineType = "c3-standard-4"
			vm.MinCpuPlatform = "Intel Sapphire Rapids"
			disks := []*compute.Disk{
				{Name: vm.Name, Type: imagetest.PdBalanced, Zone: dedicatedZone},
			}
			tvm, err := t.CreateTestVMFromInstanceBeta(vm, disks)
			if err != nil {
				return err
			}
			tvm.ForceZone(dedicatedZone)
			tvm.RunTests(tdxtests)
		}
	}
	return nil
}
