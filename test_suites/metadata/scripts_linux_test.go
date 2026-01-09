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

//go:build linux

package metadata

import (
	"fmt"
	"path"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// TestShutdownURLScripts verifies that the shutdown script URL metadata works as expected.
func TestShutdownURLScripts(t *testing.T) {
	testScripts(t, "shutdown", true)
}

// testScripts tests the scripts for the given stage and type.
// stage is one of "startup" or "shutdown".
func testScripts(t *testing.T, stage string, success bool) {
	t.Helper()

	ctx := utils.Context(t)
	expectedContent := fmt.Sprintf("%s_success", stage)
	result, err := utils.GetMetadata(ctx, "instance", "guest-attributes", "testing", "result")
	if err != nil {
		t.Fatalf("failed to read startup script result key: %v", err)
	}
	if (result == expectedContent) != success {
		t.Fatalf(`startup script output expected to be success: %t, got %s`, success, result)
	}

	// Exceptions for certain Linux images.
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	}
	if utils.IsCOS(image) {
		return
	}

	// Clear the guest attribute for the next test.
	err = utils.PutMetadata(ctx, path.Join("instance", "guest-attributes", "testing", "result"), "")
	if err != nil {
		t.Fatalf("failed to clear startup script result: %s", err)
	}
}
