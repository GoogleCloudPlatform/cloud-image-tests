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

// Package networkinterfacenaming is a CIT suite for testing that network interface names follow an acceptable scheme.
package networkinterfacenaming

import (
	"flag"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

var (
	// Name is the name of the test package. It must match the directory name.
	Name = "networkinterfacenaming"

	// nicnamingMetalZone is the zone where the metal instance is created.
	// By default, it is set to us-central1-a. The zone must be a zone in which
	// the c3-metal machine type is available.
	//
	// Refer to https://cloud.google.com/compute/docs/general-purpose-machines#c3_regions for the list of zones.
	nicnamingMetalZone = flag.String("networkinterfacenaming_metal_zone", "us-central1-a", "The zone where the metal instance is created. For zones with availability, refer to https://cloud.google.com/compute/docs/general-purpose-machines#c3_regions.")
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	network1, err := t.CreateNetwork("network-1", false)
	if err != nil {
		return err
	}
	_, err = network1.CreateSubnetwork("subnetwork-1", "10.128.0.0/20")
	if err != nil {
		return err
	}

	network2, err := t.CreateNetwork("network-2", false)
	if err != nil {
		return err
	}
	_, err = network2.CreateSubnetwork("subnetwork-2", "192.168.0.0/16")
	if err != nil {
		return err
	}
	var nic1Type, nic2Type string
	if t.Image.Architecture != "ARM64" && utils.HasFeature(t.Image, "GVNIC") {
		// We would prefer to test both a virtio and gvnic, but ARM series
		// instances do not support virtio and we need to confirm gvnic support in
		// the image.
		// If testing a mixed type configuration is impossible we leave it up to
		// the instance to use the default NIC type.
		nic1Type = "VIRTIO_NET"
		nic2Type = "GVNIC"
	}

	nicname := &daisy.Instance{}
	nicname.NetworkInterfaces = []*compute.NetworkInterface{
		{
			NicType:    nic1Type,
			Subnetwork: "subnetwork-1",
		},
		{
			NicType:    nic2Type,
			Subnetwork: "subnetwork-2",
		},
	}
	diskType := imagetest.DiskTypeNeeded(t.MachineType.Name)
	nicnameVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "nicname", Type: diskType}}, nicname)
	if err != nil {
		return err
	}
	nicnameVM.RunTests("TestNICNamingScheme")

	if t.Image.Architecture == "X86_64" && utils.HasFeature(t.Image, "IDPF") {
		c3metal := &daisy.Instance{}
		c3metal.MachineType = "c3-standard-192-metal"
		c3metal.Zone = *nicnamingMetalZone
		c3metal.Scheduling = &compute.Scheduling{OnHostMaintenance: "TERMINATE"}
		c3MetalDiskType := imagetest.DiskTypeNeeded(c3metal.MachineType)
		c3metalVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "c3metal", Type: c3MetalDiskType, Zone: *nicnamingMetalZone}}, c3metal)
		if err != nil {
			return err
		}
		c3metalVM.RunTests("TestIDPFNICNamingScheme")
	}

	return nil
}
