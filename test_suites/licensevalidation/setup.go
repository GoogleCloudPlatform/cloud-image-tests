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

// Package licensevalidation is a CIT suite for validating that an image has
// expected licenses attached to it.
package licensevalidation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

var imageSuffixRe = regexp.MustCompile(`-(arm|amd|x86_)64$`)
var sqlWindowsVersionRe = regexp.MustCompile("windows-[0-9]{4}-dc")
var sqlVersionRe = regexp.MustCompile("sql-[0-9]{4}-(express|enterprise|standard|web)")

// Name is the name of the test package. It must match the directory name.
var Name = "licensevalidation"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	// Skipping license check for preview 2025 image and Windows Client Images. TODO: Remove with official release.
	switch {
	case strings.Contains(t.Image.Name, "windows-server-2025"):
		t.Skip("Windows Server 2025 is in preview; skipping GCE license check.")
	case strings.Contains(t.Image.Name, "windows-11"):
		t.Skip("Skipping license check for Windows 11 client images.")
	case strings.Contains(t.Image.Name, "windows-10"):
		t.Skip("Skipping license check for Windows 10 client images.")
	case strings.Contains(t.Image.Name, "opensuse-leap"):
		t.Skip("Skipping license check for opensuse-leap images.")
	case strings.Contains(t.Image.Name, "opensuse-leap-arm64"):
		t.Skip("Skipping license check for opensuse-leap-arm64 images.")
	}

	licensetests := "TestLicenses"
	if utils.HasFeature(t.Image, "WINDOWS") {
		licensetests += "|TestWindowsActivationStatus"
	}
	vm1, err := t.CreateTestVM("licensevm")
	if err != nil {
		return err
	}
	rlicenses, err := requiredLicenseList(t)
	if err != nil {
		return err
	}
	vm1.AddMetadata("expected-licenses", rollStringToString(rlicenses))
	vm1.AddMetadata("actual-licenses", rollStringToString(t.Image.Licenses))
	vm1.AddMetadata("expected-license-codes", rollInt64ToString(t.Image.LicenseCodes))
	vm1.RunTests(licensetests)
	return nil
}

func rollStringToString(list []string) string {
	var result string
	for i, item := range list {
		if i != 0 {
			result += ","
		}
		result += fmt.Sprintf("%s", item)
	}
	return result
}

func rollInt64ToString(list []int64) string {
	var result string
	for i, item := range list {
		if i != 0 {
			result += ","
		}
		result += fmt.Sprintf("%d", item)
	}
	return result
}

// generate a list of license URLs we should expect to see on the
// testworkflow's image from the Name and Family properties
func requiredLicenseList(t *imagetest.TestWorkflow) ([]string, error) {
	image := t.Image
	licenseURLTmpl := t.Client.BasePath()

	if licenseURLTmpl == "" || licenseURLTmpl == "https://compute.googleapis.com/compute/v1/" {
		licenseURLTmpl = "https://www.googleapis.com/compute/v1/"
	}
	// None of the url, file, or path Join functions handle this correctly.
	if !strings.HasSuffix(licenseURLTmpl, "/") {
		licenseURLTmpl += "/"
	}
	licenseURLTmpl += "projects/%s/global/licenses/%s"
	transform := func() {}
	var requiredLicenses []string
	var project string
	switch {
	case strings.Contains(image.Name, "debian"):
		project = "debian-cloud"
		transform = func() {
			// Rightmost dash separated segment with only [a-z] chars should be the codename
			var codename string
			segments := strings.Split(image.Name, "-")
			for i := len(segments) - 1; i >= 0; i-- {
				if len(regexp.MustCompile("[a-z]+").FindString(segments[i])) == len(segments[i]) {
					codename = segments[i]
					break
				}
			}
			for i := range requiredLicenses {
				requiredLicenses[i] += "-" + codename
			}
		}
	case strings.Contains(image.Name, "rhel") && strings.Contains(image.Name, "sap"):
		project = "rhel-sap-cloud"
		transform = func() {
			newSuffix := "-sap"
			if strings.Contains(image.Name, "byos") {
				newSuffix += "-byos"
			}
			rhelSapVersionRe := regexp.MustCompile("-[0-9]+-sap-(ha|byos)$")
			requiredLicenses[len(requiredLicenses)-1] = rhelSapVersionRe.ReplaceAllString(requiredLicenses[len(requiredLicenses)-1], newSuffix)
		}
	case strings.Contains(image.Name, "rhel"):
		project = "rhel-cloud"
		transform = func() {
			if !strings.Contains(image.Name, "byos") {
				requiredLicenses[len(requiredLicenses)-1] += "-server"
			}
			requiredLicenses[len(requiredLicenses)-1] = strings.ReplaceAll(requiredLicenses[len(requiredLicenses)-1], "-c3m", "")
		}
	case strings.Contains(image.Name, "centos"):
		project = "centos-cloud"
		transform = func() {
			if image.Family == "centos-stream-8" {
				// centos-stream-8 doesn't include -8
				requiredLicenses[len(requiredLicenses)-1] = requiredLicenses[len(requiredLicenses)-1][:len(requiredLicenses[len(requiredLicenses)-1])-2]
			}
		}
	case strings.Contains(image.Name, "rocky") && strings.Contains(image.Name, "nvidia"):
		project = "rocky-linux-cloud"
		rockyVersion := strings.TrimPrefix(regexp.MustCompile("rocky-linux-[0-9]{1}").FindString(image.Name), "rocky-linux-")
		gpuDriverVersion := strings.TrimPrefix(regexp.MustCompile("nvidia-([0-9]{3}|latest)").FindString(image.Name), "nvidia-")
		transform = func() {
			requiredLicenses = []string{
				fmt.Sprintf(licenseURLTmpl, "rocky-linux-accelerator-cloud", fmt.Sprintf("nvidia-%s", gpuDriverVersion)),
				fmt.Sprintf(licenseURLTmpl, "rocky-linux-accelerator-cloud", fmt.Sprintf("rocky-linux-%s-accelerated", rockyVersion)),
				fmt.Sprintf(licenseURLTmpl, "rocky-linux-cloud", fmt.Sprintf("rocky-linux-%s-optimized-gcp", rockyVersion)),
			}
		}
	case strings.Contains(image.Name, "rocky-linux"):
		project = "rocky-linux-cloud"
	case strings.Contains(image.Name, "almalinux"):
		project = "almalinux-cloud"
	case strings.Contains(image.Name, "opensuse"):
		project = "opensuse-cloud"
		transform = func() { requiredLicenses[len(requiredLicenses)-1] += "-42" } // Quirk of opensuse licensing. This suffix will not need to be updated with version changes.
	case strings.Contains(image.Name, "sles") && strings.Contains(image.Name, "sap"):
		project = "suse-sap-cloud"
	case strings.Contains(image.Name, "sles"):
		project = "suse-cloud"
		transform = func() {
			for i := range requiredLicenses {
				requiredLicenses[i] = strings.TrimSuffix(requiredLicenses[i], "-sp5")
			}
		}
	case strings.Contains(image.Name, "ubuntu") && strings.Contains(image.Name, "nvidia"):
		project = "ubuntu-os-cloud"
		ubuntuVersion := strings.TrimPrefix(regexp.MustCompile("ubuntu-accelerator-[0-9]{4}").FindString(image.Name), "ubuntu-accelerator-")
		if strings.HasSuffix(ubuntuVersion, "04") {
			ubuntuVersion += "-lts"
		}
		gpuDriverVersion := strings.TrimPrefix(regexp.MustCompile("nvidia-([0-9]{3}|latest)").FindString(image.Name), "nvidia-")
		transform = func() {
			requiredLicenses[0] = fmt.Sprintf(licenseURLTmpl, "ubuntu-os-cloud", fmt.Sprintf("ubuntu-%s", ubuntuVersion))
			requiredLicenses = append(requiredLicenses,
				fmt.Sprintf(licenseURLTmpl, "ubuntu-os-accelerator-images", fmt.Sprintf("ubuntu-%s-accelerated", ubuntuVersion)),
				fmt.Sprintf(licenseURLTmpl, "ubuntu-os-accelerator-images", fmt.Sprintf("nvidia-%s", gpuDriverVersion)),
			)
		}
	case strings.Contains(image.Name, "ubuntu-pro") || strings.Contains(image.Name, "ubuntu-minimal-pro"):
		project = "ubuntu-os-pro-cloud"
	case strings.Contains(image.Name, "ubuntu"):
		project = "ubuntu-os-cloud"
	case strings.Contains(image.Name, "windows") && strings.Contains(image.Name, "sql"):
		project = "windows-cloud"
		transform = func() {
			requiredLicenses = []string{
				fmt.Sprintf(licenseURLTmpl, "windows-sql-cloud", strings.Replace(sqlVersionRe.FindString(image.Name), "sql-", "sql-server-", -1)),
				fmt.Sprintf(licenseURLTmpl, project, strings.Replace(sqlWindowsVersionRe.FindString(image.Name), "windows-", "windows-server-", -1)),
			}
		}
	case strings.Contains(image.Name, "windows"):
		project = "windows-cloud"
		transform = func() {
			requiredLicenses = []string{fmt.Sprintf(licenseURLTmpl, project, "windows-server-"+regexp.MustCompile("[0-9]{4}(-r[0-9])?").FindString(image.Family)+"-dc")}
			if strings.Contains(image.Name, "core") {
				requiredLicenses = append(requiredLicenses, fmt.Sprintf(licenseURLTmpl, project, "windows-server-core"))
			} else if strings.Contains(image.Name, "bios") {
				requiredLicenses = append(requiredLicenses, fmt.Sprintf(licenseURLTmpl, "google.com:windows-internal", "internal-windows"))
			}
		}
	default:
		return nil, fmt.Errorf("Not sure what project to look for licenses from for %s", image.Name)
	}

	requiredLicenses = append(requiredLicenses, fmt.Sprintf(licenseURLTmpl, project, imageSuffixRe.ReplaceAllString(image.Family, "")))
	transform()

	return requiredLicenses, nil
}
