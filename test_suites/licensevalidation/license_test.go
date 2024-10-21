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

package licensevalidation

import (
	"sort"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func TestWindowsActivationStatus(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata %v", err)
	}
	if utils.IsWindowsClient(image) {
		t.Skip("Activation status only checked on server images.")
	}

	command := "cscript C:\\Windows\\system32\\slmgr.vbs /dli"
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Error getting license status: %v", err)
	}

	if !strings.Contains(output.Stdout, "License Status: Licensed") {
		t.Fatalf("Activation info does not contain 'Licensed': %s", output.Stdout)
	}
}

func TestLicenses(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	elicensecodes, err := utils.GetMetadata(ctx, "instance", "attributes", "expected-license-codes")
	if err != nil {
		t.Fatalf("Failed to get expected licenses: %v", err)
	}
	expectedLicenseCodes := strings.Split(elicensecodes, ",")
	var actualLicenseCodes []string
	licenseNums, err := utils.GetMetadata(ctx, "instance", "licenses")
	if err != nil {
		t.Fatalf("could not get instance licenses: %v", err)
	}
	for _, lnum := range strings.Split(licenseNums, "\n") {
		lnum = strings.TrimSpace(lnum)
		if lnum == "" {
			continue
		}
		id, err := utils.GetMetadata(ctx, "instance", "licenses", lnum, "id")
		if err != nil {
			t.Fatalf("could not get license %s id: %v", lnum, err)
		}
		actualLicenseCodes = append(actualLicenseCodes, id)
	}
	elicenses, err := utils.GetMetadata(ctx, "instance", "attributes", "expected-licenses")
	if err != nil {
		t.Fatalf("Failed to get expected licenses: %v", err)
	}
	expectedLicenses := strings.Split(elicenses, ",")
	alicenses, err := utils.GetMetadata(ctx, "instance", "attributes", "actual-licenses")
	if err != nil {
		t.Fatalf("Failed to get actual licenses: %v", err)
	}
	actualLicenses := strings.Split(alicenses, ",")

	sort.Strings(expectedLicenseCodes)
	sort.Strings(actualLicenseCodes)
	if len(expectedLicenseCodes) != len(actualLicenseCodes) {
		t.Errorf("wrong number of license codes, got %d want %d", len(actualLicenseCodes), len(expectedLicenseCodes))
	}
	for i := range expectedLicenseCodes {
		if expectedLicenseCodes[i] != actualLicenseCodes[i] {
			t.Errorf("unexpected license code at pos %d, got %s want %s", i, actualLicenseCodes[i], expectedLicenseCodes[i])
		}
	}

	sort.Strings(expectedLicenses)
	sort.Strings(actualLicenses)
	if len(expectedLicenses) != len(actualLicenses) {
		t.Errorf("wrong number of licenses, got %d want %d", len(actualLicenses), len(expectedLicenses))
	}
	for i := range expectedLicenses {
		if expectedLicenses[i] != actualLicenses[i] {
			t.Errorf("unexpected license at pos %d, got %s want %s", i, actualLicenses[i], expectedLicenses[i])
		}
	}
}
