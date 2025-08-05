// Copyright 2025 Google LLC.
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

//go:build windows

package metadata

import (
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// testScripts tests the scripts for the given stage and type.
// stage is one of "startup", "shutdown", or "sysprep".
func testScripts(t *testing.T, stage string, success bool) {
	t.Helper()

	tests := []struct {
		name           string
		guestAttribute string
	}{
		{
			name:           fmt.Sprintf("%s-ps1-test-%t", stage, success),
			guestAttribute: fmt.Sprintf("%s-ps1-result", stage),
		},
		{
			name:           fmt.Sprintf("%s-cmd-test-%t", stage, success),
			guestAttribute: fmt.Sprintf("%s-cmd-result", stage),
		},
		{
			name:           fmt.Sprintf("%s-bat-test-%t", stage, success),
			guestAttribute: fmt.Sprintf("%s-bat-result", stage),
		},
	}
	// Only startup scripts can't run URL test.
	if stage == "sysprep" || stage == "shutdown" {
		tests = append(tests, struct {
			name           string
			guestAttribute string
		}{
			name:           fmt.Sprintf("%s-url-test-%t", stage, success),
			guestAttribute: fmt.Sprintf("%s-url-result", stage),
		})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res, err := utils.GetMetadata(utils.Context(t), "instance", "guest-attributes", "testing", test.guestAttribute)
			if err != nil {
				t.Fatalf("couldn't get %s from metadata, %v", test.guestAttribute, err)
			}

			if strings.Contains(res, "success") != success {
				t.Errorf("expected %s to be success: %t, got %s", test.guestAttribute, success, res)
			}

			// Clear the guest attribute.
			if err := utils.PutMetadata(utils.Context(t), path.Join("instance", "guest-attributes", "testing", test.guestAttribute), ""); err != nil {
				t.Fatalf("failed to clear %s: %v", test.guestAttribute, err)
			}
		})
	}
}

func TestSysprepSpecialize(t *testing.T) {
	testScripts(t, "sysprep", true)
}
