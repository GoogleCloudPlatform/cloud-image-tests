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
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	// //T: makes cscript kill slmgr.vbs after 120 s so a wedged licensing
	// service cannot hang the activation retry loop.
	slmgrCmd = `cscript //Nologo //T:120 C:\Windows\system32\slmgr.vbs`
	// KMS activations are severed when a backend KMS host stalls, so a first
	// boot activation attempt can fail transiently. Retry activation for a
	// while before declaring the image broken.
	activationRetryInterval = 20 * time.Second
	activationRetryTimeout  = 10 * time.Minute
)

// slmgr runs slmgr.vbs with args and returns its combined stdout and stderr.
// On failure the returned error also carries that output.
func slmgr(args string) (string, error) {
	out, err := utils.RunPowershellCmd(slmgrCmd + " " + args)
	text := strings.TrimSpace(out.Stdout + "\n" + out.Stderr)
	if err != nil {
		return text, fmt.Errorf("%w: %s", err, text)
	}
	return text, nil
}

// windowsLicenseStatus reports whether Windows is activated and the
// `slmgr /dli` output (or error) for diagnostics.
func windowsLicenseStatus() (bool, string) {
	out, err := slmgr("/dli")
	if err != nil {
		return false, "slmgr /dli: " + err.Error()
	}
	return strings.Contains(out, "License Status: Licensed"), "slmgr /dli: " + out
}

func TestWindowsActivationStatus(t *testing.T) {
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata %v", err)
	}
	if utils.IsWindowsClient(image) {
		t.Skip("Activation status only checked on server images.")
	}

	licensed, status := windowsLicenseStatus()
	if licensed {
		return
	}
	t.Logf("Windows was not activated on first boot, retrying activation: %s", status)
	err = utils.PollUntil(ctx, "Windows activation", activationRetryInterval, activationRetryTimeout, func() (bool, string) {
		ato, atoErr := slmgr("/ato")
		if atoErr != nil {
			ato = atoErr.Error()
		}
		licensed, status := windowsLicenseStatus()
		if licensed {
			return true, ""
		}
		return false, "slmgr /ato: " + ato + "; " + status
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLicenses(t *testing.T) {
	ctx := utils.Context(t)
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
