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

// Package hotattach is a CIT suite from testing hot attaching/detaching
package hotattach

import (
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "hotattach"

const (
	// the path to write the file on linux
	linuxMountPath          = "/mnt/disks/hotattach"
	mkfsCmd                 = "mkfs.ext4"
	windowsMountDriveLetter = "F"
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	hotattachInst := &daisy.Instance{}
	hotattachInst.Scopes = append(hotattachInst.Scopes, "https://www.googleapis.com/auth/cloud-platform")

	diskType := imagetest.DiskTypeNeeded(t.MachineType.Name)

	hotattach, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: strings.Replace("reattach"+diskType, "-", "", -1), Type: diskType}, {Name: "hotattachmount", Type: diskType, SizeGb: 30}}, hotattachInst)
	if err != nil {
		return err
	}
	hotattach.AddMetadata("hotattach-disk-name", "hotattachmount")
	hotattach.RunTests("TestFileHotAttach")
	return nil
}
