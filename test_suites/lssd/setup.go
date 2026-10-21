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

// Package lssd is a CIT suite from testing mounting/umounting of disks.
package lssd

import (
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"slices"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "lssd"

// c3LSSDZones lists the zones that support c3-standard-8-lssd and have C3_CPUS
// and Local SSD quota in the CIT test pool projects.
var c3LSSDZones = []string{
	"us-central1-a", "us-central1-b", "us-central1-c",
	"us-east1-b", "us-east1-c", "us-east1-d",
	"us-east4-a", "us-east4-b", "us-east4-c",
	"us-west1-a", "us-west1-b",
	"us-west4-a", "us-west4-b",
	"europe-west1-b", "europe-west1-c", "europe-west1-d",
	"europe-west2-a", "europe-west2-c",
	"europe-west3-a", "europe-west3-b", "europe-west3-c",
	"europe-west4-a", "europe-west4-b", "europe-west4-c",
	"asia-southeast1-a", "asia-southeast1-b", "asia-southeast1-c",
}

// pickZone returns zone if it is in zones, and a random zone from zones
// otherwise. CIT reruns TestSetup with the next -zones entry after a stockout or
// quota error, so a retry picks again.
func pickZone(zone string, zones []string) string {
	if slices.Contains(zones, zone) {
		return zone
	}
	return zones[rand.IntN(len(zones))]
}

var (
	// lssdMetalMachineType is an optional Bare Metal LSSD machine type (e.g.
	// z3-highmem-192-highlssd-metal, c4d-standard-384-lssd-metal, or
	// a5x-highgpu-4g-metal) used to test Bare Metal Local SSD udev naming
	// end-to-end.
	lssdMetalMachineType = flag.String("lssd_metal_machine_type", "", "Optional Bare Metal LSSD machine type to provision for Bare Metal LSSD udev naming tests.")

	// lssdMetalZone is the optional zone where the Bare Metal LSSD instance is
	// created. Defaults to the workflow zone.
	lssdMetalZone = flag.String("lssd_metal_zone", "", "Optional zone where the Bare Metal LSSD instance is created.")
)

const (
	// the path to write the file on linux
	linuxMountPath          = "/mnt/disks/hotattach"
	mkfsCmd                 = "mkfs.ext4"
	windowsMountDriveLetter = "F"
	vmLSSDTests             = "TestMount|TestUdevRulesAndScript|TestLSSDSymlinksAndUdevProperties|TestLSSDPartitionSymlinks|TestLSSDGoogleNVMeIDNaming"
	bareMetalLSSDTests      = "TestBareMetalMount|TestBareMetalUdevRulesAndScript|TestBareMetalLSSDSymlinksAndUdevProperties|TestBareMetalLSSDPartitionSymlinks|TestBareMetalLSSDVendorExtensionNaming|TestBareMetalLSSDSerialOrderingNaming"
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	var zone string
	if t.Image.Architecture != "ARM64" && utils.HasFeature(t.Image, "GVNIC") && t.ShouldRunTests(vmLSSDTests) {
		zone = pickZone(t.Zone.Name, c3LSSDZones)
		log.Printf("Using zone %s for c3-standard-8-lssd test", zone)
		lssdMountInst := &daisy.Instance{}
		lssdMountInst.Zone = zone
		lssdMountInst.MachineType = "c3-standard-8-lssd"

		lssdMount, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Zone: zone, Name: "remountLSSD", Type: imagetest.PdBalanced}}, lssdMountInst)
		if err != nil {
			return err
		}
		// local SSD's don't show up exactly as their device name under /dev/disk/by-id
		if utils.HasFeature(t.Image, "WINDOWS") {
			lssdMount.AddMetadata("hotattach-disk-name", "nvme_card0")
		} else {
			lssdMount.AddMetadata("hotattach-disk-name", "local-nvme-ssd-0")
		}
		lssdMount.RunTests(vmLSSDTests)
	}

	metalMachineType := *lssdMetalMachineType
	if metalMachineType == "" && imagetest.IsMetal(t.MachineType.Name) && t.MachineType.BundledLocalSsds != nil && t.MachineType.BundledLocalSsds.PartitionCount > 0 {
		metalMachineType = t.MachineType.Name
	}
	if metalMachineType != "" && !utils.HasFeature(t.Image, "WINDOWS") && (utils.HasFeature(t.Image, "GVNIC") || utils.HasFeature(t.Image, "IDPF")) && t.ShouldRunTests(bareMetalLSSDTests) {
		metalZone := *lssdMetalZone
		if metalZone == "" {
			metalZone = t.Zone.Name
		}
		if metalZone == "" {
			metalZone = zone
		}
		fmt.Printf("Using zone %s for %s Bare Metal LSSD instance\n", metalZone, metalMachineType)
		lssdMetalInst := &daisy.Instance{}
		lssdMetalInst.Zone = metalZone
		lssdMetalInst.MachineType = metalMachineType
		lssdMetalInst.Scheduling = &compute.Scheduling{OnHostMaintenance: imagetest.MachineMaintenancePolicy(metalMachineType)}
		metalDiskType := imagetest.DiskTypeNeeded(metalMachineType)

		lssdMetal, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Zone: metalZone, Name: "lssdMetal", Type: metalDiskType}}, lssdMetalInst)
		if err != nil {
			return err
		}
		lssdMetal.AddMetadata("hotattach-disk-name", "local-nvme-ssd-0")
		lssdMetal.RunTests(bareMetalLSSDTests)
	}
	return nil
}
