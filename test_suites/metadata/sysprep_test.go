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
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func TestSysprepSpecialize(t *testing.T) {
	utils.WindowsOnly(t)
	tests := []struct {
		name           string
		key            string
		expectedResult string
	}{
		{
			name:           "TestSysprepSpecializePs1",
			key:            "sysprep-ps1-result",
			expectedResult: "sysprep_ps1_success",
		},
		{
			name:           "TestSysprepSpecializeCmd",
			key:            "sysprep-cmd-result",
			expectedResult: "sysprep_cmd_success",
		},
		{
			name:           "TestSysprepSpecializeBat",
			key:            "sysprep-bat-result",
			expectedResult: "sysprep_bat_success",
		},
		{
			name:           "TestSysprepSpecializePs1Url",
			key:            "sysprep-ps1-url-result",
			expectedResult: "sysprep_ps1-url_success",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := utils.GetMetadata(utils.Context(t), "instance", "guest-attributes", "testing", test.key)
			if err != nil {
				t.Fatalf("failed to read sysprep script result key %s: %v", test.key, err)
			}
			if result != test.expectedResult {
				t.Fatalf("sysprep-specialize script output expected %q, got %q", test.expectedResult, result)
			}
		})
	}
}
