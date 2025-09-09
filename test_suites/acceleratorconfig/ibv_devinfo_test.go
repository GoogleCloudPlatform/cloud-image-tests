// Copyright 2024 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package acceleratorconfig

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func installIbvUtils(ctx context.Context, t *testing.T) {
	t.Helper()
	// Rocky Linux accelerator OS already contains ibv_devinfo
	if isRockyLinux(ctx, t) {
		return
	}
	if out, err := exec.CommandContext(ctx, "apt", "update").CombinedOutput(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, apt update).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", err, out)
	}
	if out, err := exec.CommandContext(ctx, "apt", "install", "-y", "ibverbs-utils").CombinedOutput(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, apt install, -y, ibverbs-utils).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", err, out)
	}
}

func TestIbvDevinfo(t *testing.T) {
	ctx := utils.Context(t)
	installIbvUtils(ctx, t)

	out, err := exec.CommandContext(ctx, "ibv_devinfo", "--verbose").CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, ibv_devinfo --verbose).CombinedOutput() = failed unexpectedly; err = %v\noutput: %s", err, out)
	}

	devices := strings.Split(string(out), "hca_id:")
	if len(devices) > 0 && devices[0] == "" {
		devices = devices[1:]
	}
	if len(devices) != 8 {
		t.Errorf("Expected 8 devices, got %d", len(devices))
	}

	for i, device := range devices {
		t.Logf("ibv_devinfo device %d output:\n%s", i, device)
		if !strings.Contains(device, "PORT_ACTIVE") {
			t.Errorf("Output for device %d  does not contain `PORT_ACTIVE`.", i)
		}
		if !strings.Contains(device, "LINK_UP") {
			t.Errorf("Output for device %d does not contain `LINK_UP`", i)
		}
	}
}
