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

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "lssd"

var (
	// lssdMetalMachineType is an optional Bare Metal LSSD machine type (e.g.
	// z3-highmem-192-highlssd-metal or c4d-standard-384-lssd-metal) used to test
	// Bare Metal Local SSD udev vendor-extension naming end-to-end.
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
	bareMetalLSSDTests      = "TestBareMetalMount|TestBareMetalUdevRulesAndScript|TestBareMetalLSSDSymlinksAndUdevProperties|TestBareMetalLSSDPartitionSymlinks|TestBareMetalLSSDVendorExtensionNaming"
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if t.Image.Architecture != "ARM64" && utils.HasFeature(t.Image, "GVNIC") {
		vmZone := t.Zone.Name
		if vmZone == "" {
			vmZone = "us-central1-a"
		}
		lssdMountInst := &daisy.Instance{}
		lssdMountInst.Zone = vmZone
		lssdMountInst.MachineType = "c3-standard-8-lssd"

		lssdMount, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Zone: vmZone, Name: "remountLSSD", Type: imagetest.PdBalanced}}, lssdMountInst)
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

		metalMachineType := *lssdMetalMachineType
		if metalMachineType == "" && imagetest.IsMetal(t.MachineType.Name) {
			metalMachineType = t.MachineType.Name
		}
		if metalMachineType != "" && !utils.HasFeature(t.Image, "WINDOWS") {
			metalZone := *lssdMetalZone
			if metalZone == "" {
				metalZone = vmZone
			}
			fmt.Printf("Using zone %s for %s Bare Metal LSSD instance\n", metalZone, metalMachineType)
			lssdMetalInst := &daisy.Instance{}
			lssdMetalInst.Zone = metalZone
			lssdMetalInst.MachineType = metalMachineType
			lssdMetalInst.Scheduling = &compute.Scheduling{OnHostMaintenance: "TERMINATE"}
			metalDiskType := imagetest.DiskTypeNeeded(metalMachineType)

			lssdMetal, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Zone: metalZone, Name: "lssdMetal", Type: metalDiskType}}, lssdMetalInst)
			if err != nil {
				return err
			}
			lssdMetal.AddMetadata("hotattach-disk-name", "local-nvme-ssd-0")
			lssdMetal.RunTests(bareMetalLSSDTests)
		}
	}
	return nil
}
