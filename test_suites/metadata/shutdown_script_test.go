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

package metadata

import (
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	expectedShutdownContent = "shutdown_success"
	// The designed shutdown limit is 90s. Let's verify it's executed no less than 80s.
	shutdownTime = 80
)

// TestShutdownScriptFailedLinux tests that a script failed execute doesn't crash the vm.
func testShutdownScriptFailedLinux(t *testing.T) error {
	if _, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "shutdown-script"); err != nil {
		return fmt.Errorf("couldn't get shutdown-script from metadata")
	}

	return nil

}

// TestShutdownScriptFailedWindows tests that a script failed execute doesn't crash the vm.
func testShutdownScriptFailedWindows(t *testing.T) error {
	if _, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "windows-shutdown-script-ps1"); err != nil {
		return fmt.Errorf("couldn't get windows-shutdown-script-ps1 from metadata")
	}

	return nil

}

// TestShutdownScripts verifies that the standard metadata script could run successfully
// by checking the output content of the Shutdown script. It also checks that
// shutdown scripts don't run on reinstall or upgrade of the guest-agent.
func TestShutdownScripts(t *testing.T) {
	ctx := utils.Context(t)
	result, err := utils.GetMetadata(ctx, "instance", "guest-attributes", "testing", "result")
	if err != nil {
		t.Fatalf("failed to read shutdown script result key: %v", err)
	}
	if result != expectedShutdownContent {
		t.Errorf(`shutdown script output expected "%s", got "%s".`, expectedShutdownContent, result)
	}
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	} else if strings.Contains(image, "sles") || strings.Contains(image, "suse") {
		t.Skipf("image %s has known issues with metadata scripts on reinstall", image)
	}
	err = utils.PutMetadata(ctx, path.Join("instance", "guest-attributes", "testing", "result"), "")
	if err != nil {
		t.Fatalf("failed to clear shutdown script result: %s", err)
	}

	reinstallGuestAgent(ctx, t)

	result, err = utils.GetMetadata(ctx, "instance", "guest-attributes", "testing", "result")
	if err != nil {
		t.Fatalf("failed to read shutdown script result key: %v", err)
	}
	if result == expectedShutdownContent {
		t.Errorf("shutdown script executed after a reinstall of guest agent")
	}
}

// Determine if the OS is Windows or Linux and run the appropriate failure test.
func TestShutdownScriptsFailed(t *testing.T) {
	if utils.IsWindows() {
		if err := testShutdownScriptFailedWindows(t); err != nil {
			t.Fatalf("Shutdown script failure test failed with error: %v", err)
		}
	} else {
		if err := testShutdownScriptFailedLinux(t); err != nil {
			t.Fatalf("Shutdown script failure test failed with error: %v", err)
		}
	}
}

// Determine if the OS is Windows or Linux and run the appropriate daemon test.
func TestShutdownURLScripts(t *testing.T) {
	result, err := utils.GetMetadata(utils.Context(t), "instance", "guest-attributes", "testing", "result")
	if err != nil {
		t.Fatalf("failed to read shutdown script result key: %v", err)
	}
	if result != expectedShutdownContent {
		t.Fatalf(`shutdown script output expected "%s", got "%s".`, expectedShutdownContent, result)
	}
}
