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

package disk

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func TestBlockDeviceNaming(t *testing.T) {
	utils.LinuxOnly(t)
	err := exec.Command("udevadm", "trigger").Run()
	if err != nil {
		t.Fatal(err)
	}
	err = exec.Command("udevadm", "settle").Run()
	if err != nil {
		t.Fatal(err)
	}
	disks, err := os.ReadDir("/dev/disk/by-id")
	if err != nil {
		t.Fatal(err)
	}
	var disklist []string
	for _, disk := range disks {
		disklist = append(disklist, disk.Name())
		if disk.Name() == "google-secondary" {
			return
		}
	}
	t.Fatalf("could not find a disk named google-secondary, found these disks: %s", strings.Join(disklist, " "))
}
