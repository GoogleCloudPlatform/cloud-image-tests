// Copyright 2026 Google LLC.
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

package packagevalidation

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

var (
	// Publish timestamp suffixes at the end (e.g. -v20250909) or before arch on SUSE images (e.g. -v20260204-x86-64).
	trailingPublishSuffixRegex = regexp.MustCompile(`-v[0-9]+[a-z]*$`)
	susePublishSuffixRegex     = regexp.MustCompile(`-v[0-9]{8}(-(?:x86-64|arm64))$`)
	// Pinned service-pack or minor versions in image names (e.g. sles-15-sp6, rhel-9-8-eus, sles-16-0, opensuse-leap-15-6).
	spVersionRegex    = regexp.MustCompile(`-([0-9]+-sp[0-9]+)(?:-|$)`)
	minorVersionRegex = regexp.MustCompile(`^[a-z-]+-([0-9]{1,2}-[0-9]{1,2})(?:-|$)`)
	windowsImageRegex = regexp.MustCompile(`(?:^|-)(?:windows(?:-server)?|win)-([0-9]{4}(?:-r[0-9])?|10|11)(?:-|$)`)
)

func TestImageVersion(t *testing.T) {
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("Failed to get image metadata: %v", err)
	}
	prefix, err := extractImagePrefix(image)
	if err != nil {
		t.Fatalf("Failed to parse image name %q: %v", image, err)
	}
	t.Logf("Validating image %q (prefix: %q)", image, prefix)

	if utils.IsWindows() {
		validateWindowsVersion(t, prefix)
		return
	}
	validateLinuxVersion(t, prefix)
}

func extractImagePrefix(image string) (string, error) {
	parts := strings.Split(strings.TrimSpace(image), "/")
	name := strings.ToLower(parts[len(parts)-1])
	prefix := trailingPublishSuffixRegex.ReplaceAllString(name, "")
	prefix = susePublishSuffixRegex.ReplaceAllString(prefix, "$1")
	if prefix == "" {
		return "", fmt.Errorf("malformed image metadata %q", image)
	}
	return prefix, nil
}

func parseOSReleaseVersionID(content string) string {
	for _, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(strings.TrimRight(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "VERSION_ID" {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

// hasToken checks whether tok appears as a complete dash-delimited segment in prefix.
func hasToken(prefix, tok string) bool {
	if tok == "" {
		return false
	}
	return strings.Contains("-"+prefix+"-", "-"+strings.ToLower(tok)+"-")
}

func validateLinuxVersion(t *testing.T, prefix string) {
	t.Helper()
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Fatalf("Failed to read /etc/os-release: %v", err)
	}
	versionID := parseOSReleaseVersionID(string(data))
	t.Logf("Parsed /etc/os-release VERSION_ID=%q", versionID)
	if versionID == "" {
		t.Fatalf("VERSION_ID not found in /etc/os-release")
	}

	if !regexp.MustCompile(`[0-9]`).MatchString(prefix) {
		t.Skipf("Image prefix %q has no version number to validate", prefix)
	}

	parts := strings.Split(versionID, ".")
	major := parts[0]
	dashed := strings.ReplaceAll(versionID, ".", "-")
	dotless := strings.ReplaceAll(versionID, ".", "")

	if m := spVersionRegex.FindStringSubmatch(prefix); len(m) == 2 {
		if len(parts) < 2 || major+"-sp"+parts[1] != m[1] {
			t.Fatalf("OS VERSION_ID %q does not match service pack %q in image prefix %q", versionID, m[1], prefix)
		}
		return
	}
	if m := minorVersionRegex.FindStringSubmatch(prefix); len(m) == 2 {
		if dashed != m[1] {
			t.Fatalf("OS VERSION_ID %q does not match minor version %q in image prefix %q", versionID, m[1], prefix)
		}
		return
	}

	if !hasToken(prefix, dashed) && !hasToken(prefix, dotless) && !hasToken(prefix, major) {
		t.Fatalf("OS VERSION_ID %q does not match image prefix %q", versionID, prefix)
	}
}

func validateWindowsVersion(t *testing.T, prefix string) {
	t.Helper()
	out, err := utils.RunPowershellCmd("(Get-CimInstance Win32_OperatingSystem).Caption")
	if err != nil {
		t.Fatalf("Failed to get Windows OS Caption: %v (stderr: %s)", err, out.Stderr)
	}
	caption := strings.TrimSpace(out.Stdout)
	t.Logf("Windows OS Caption=%q", caption)

	m := windowsImageRegex.FindStringSubmatch(prefix)
	if len(m) != 2 {
		t.Skipf("Image prefix %q does not specify a Windows version number", prefix)
	}
	expected := strings.ReplaceAll(m[1], "-", " ")
	captionLower := strings.ToLower(caption)
	if !strings.Contains(captionLower, "windows server "+expected) && !strings.Contains(captionLower, "windows "+expected) {
		t.Fatalf("Windows OS Caption %q does not match expected version %q from image prefix %q", caption, expected, prefix)
	}
}
