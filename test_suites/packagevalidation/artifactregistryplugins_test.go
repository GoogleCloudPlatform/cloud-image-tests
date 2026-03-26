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
	"os/exec"
	"strings"
	"testing"
)

const (
	// These are authenticated repos used for testing artifact registry plugin.
	elTemplate = `
[google-compute-engine-%s-x86-64-testing]
name=google-compute-engine-%s-x86-64-testing
baseurl=https://packages.cloud.google.com/yum/repos/google-compute-engine-%s-x86_64-testing
enabled=1
gpgcheck=1
repo_gpgcheck=0
artifact_registry_oauth=1`

	aptTemplate = `
deb ar+https://packages.cloud.google.com/apt google-compute-engine-%s-testing main
`
	yumRepoPath = "/etc/yum.repos.d/guest-env-testing.repo"
	aptRepoPath = "/etc/apt/sources.list.d/guest-env-testing.list"
)

func imageVersion(t *testing.T) (string, string, string) {
	t.Helper()
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Fatalf("Failed to read /etc/os-release: %v", err)
	}

	var osID, versionID, versionCodename string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			value := strings.Trim(parts[1], "\"")
			switch key {
			case "ID":
				osID = value
			case "VERSION_ID":
				versionID = value
			case "VERSION_CODENAME":
				versionCodename = value
			}
		}
	}

	if osID == "debian" {
		if versionCodename != "" {
			return versionCodename, aptRepoPath, aptTemplate
		}
		t.Fatalf("Debian system detected but VERSION_CODENAME not found in /etc/os-release")
	}

	if versionID != "" {
		majorVersion := strings.Split(versionID, ".")[0]
		return fmt.Sprintf("el%s", majorVersion), yumRepoPath, elTemplate
	}

	t.Fatalf("Failed to determine OS version from /etc/os-release")
	return "", "", ""
}

func run(t *testing.T, name string, arg ...string) string {
	t.Helper()
	cmd := exec.Command(name, arg...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run command: %s %v: %v, output: %s", name, arg, err, string(output))
	}
	outStr := string(output)
	t.Logf("Successfully ran command: %s %v, output:\n%s", name, arg, outStr)
	return outStr
}

func addTestingRepo(t *testing.T, imgVer, repoPath, template string) {
	t.Helper()
	var config string
	if repoPath == aptRepoPath {
		if imgVer == "trixie" {
			template = strings.Replace(template, "deb ar+", "deb [signed-by=/etc/apt/keyrings/google-keyring.gpg] ar+", 1)
		}
		config = fmt.Sprintf(template, imgVer)
	} else {
		config = fmt.Sprintf(template, imgVer, imgVer, imgVer)
	}

	err := os.WriteFile(repoPath, []byte(config), 0644)
	if err != nil {
		t.Fatalf("Failed to write repo config: %v", err)
	}
}

func TestArtifactRegistryPlugin(t *testing.T) {
	imgVer, repoPath, template := imageVersion(t)
	if repoPath == aptRepoPath {
		addTestingRepo(t, imgVer, repoPath, template)
		run(t, "apt-get", "update")
		output := run(t, "apt-get", "download", "google-guest-agent")
		if !strings.Contains(output, imgVer+"-testing") {
			t.Errorf("apt-get download output does not contain \"%s-testing\", download may not be from testing repo. output: %s", imgVer, output)
		}
	} else if repoPath == yumRepoPath {
		addTestingRepo(t, imgVer, repoPath, template)
		run(t, "dnf", "makecache")
		run(t, "dnf", "download", "google-guest-agent", fmt.Sprintf("--repo=google-compute-engine-%s-x86-64-testing", imgVer))
	}
}
