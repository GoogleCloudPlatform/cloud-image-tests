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
	"log"
	"math/rand/v2"
	"slices"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "cvm"

// cvmZones lists the zones that support every CVM type tested here: SEV on C3D
// machines (AMD Genoa CPU), SEV-SNP on N2D machines (AMD Milan CPU), and Intel TDX
// on C3 machines (Intel Sapphire Rapids CPU).
// See https://docs.cloud.google.com/confidential-computing/confidential-vm/docs/supported-configurations#supported-zones
var cvmZones = []string{
	"us-central1-a", "us-central1-b", "us-central1-c",
	"europe-west3-a", "europe-west3-b",
	"europe-west4-a", "europe-west4-b", "europe-west4-c",
	"asia-southeast1-a", "asia-southeast1-b", "asia-southeast1-c",
}

// pickZone returns zone if it is in cvmZones, and a random zone from cvmZones
// otherwise. CIT reruns TestSetup with the next -zones entry after a stockout or
// quota error, so a retry picks again.
func pickZone(zone string) string {
	if slices.Contains(cvmZones, zone) {
		return zone
	}
	return cvmZones[rand.IntN(len(cvmZones))]
}

// TestSetup sets up test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	for _, feature := range t.ImageBeta.GuestOsFeatures {
		switch feature.Type {
		case "SEV_CAPABLE":
			zone := pickZone(t.Zone.Name)
			log.Printf("Using zone %s for SEV test", zone)
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
			vm.MachineType = "c3d-standard-4"
			vm.MinCpuPlatform = "AMD Genoa"
			disks := []*compute.Disk{{Name: vm.Name, Type: imagetest.PdBalanced, Zone: zone}}
			tvm, err := t.CreateTestVMFromInstanceBeta(vm, disks)
			if err != nil {
				return err
			}
			tvm.ForceZone(zone)
			tvm.RunTests(sevtests)
		case "SEV_SNP_CAPABLE":
			zone := pickZone(t.Zone.Name)
			log.Printf("Using zone %s for SEV-SNP test", zone)
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
				{Name: vm.Name, Type: imagetest.PdBalanced, Zone: zone},
			}
			tvm, err := t.CreateTestVMFromInstanceBeta(vm, disks)
			if err != nil {
				return err
			}
			tvm.ForceZone(zone)
			tvm.RunTests(sevsnptests)
		case "TDX_CAPABLE":
			zone := pickZone(t.Zone.Name)
			log.Printf("Using zone %s for TDX test", zone)
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
				{Name: vm.Name, Type: imagetest.PdBalanced, Zone: zone},
			}
			tvm, err := t.CreateTestVMFromInstanceBeta(vm, disks)
			if err != nil {
				return err
			}
			tvm.ForceZone(zone)
			tvm.RunTests(tdxtests)
		}
	}
	return nil
}
