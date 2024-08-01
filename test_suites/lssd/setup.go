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
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "lssd"

const (
	bootDiskSizeGB = 10

	// the path to write the file on linux
	linuxMountPath          = "/mnt/disks/hotattach"
	mkfsCmd                 = "mkfs.ext4"
	windowsMountDriveLetter = "F"
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if t.Image.Architecture != "ARM64" && utils.HasFeature(t.Image, "GVNIC") {
		lssdMountInst := &daisy.Instance{}
		lssdMountInst.Zone = "us-central1-a"
		lssdMountInst.MachineType = "c3-standard-8-lssd"

		lssdMount, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Zone: "us-central1-a", Name: "remountLSSD", Type: imagetest.PdBalanced, SizeGb: bootDiskSizeGB}}, lssdMountInst)
		if err != nil {
			return err
		}
		// local SSD's don't show up exactly as their device name under /dev/disk/by-id
		if utils.HasFeature(t.Image, "WINDOWS") {
			lssdMount.AddMetadata("hotattach-disk-name", "nvme_card0")
		} else {
			lssdMount.AddMetadata("hotattach-disk-name", "local-nvme-ssd-0")
		}
		lssdMount.RunTests("TestMount")
	}
	return nil
}
