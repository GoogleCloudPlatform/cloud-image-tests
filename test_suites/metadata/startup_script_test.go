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
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// scriptData contains the data extracted from a startup script URL.
type scriptData struct {
	// pattern is the string identifying the URL pattern.
	pattern string
	// bucket is the gcs bucket from the URL.
	bucket string
	// object is the gcs object from the URL.
	object string
}

const (
	gsPattern                        = "GS"
	httpStoragePattern               = "HTTP_STORAGE"
	httpStorageCloudPattern          = "HTTP_STORAGE_CLOUD"
	httpStorageCommondataPattern     = "HTTP_STORAGE_COMMONDATA"
	httpStorageCloudGoogleComPattern = "HTTP_STORAGE_CLOUD_GOOGLE_COM"
)

// TestStartupScriptFailedLinux tests that a script failed execute doesn't crash the vm.
func testStartupScriptFailedLinux(t *testing.T) error {
	if _, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "startup-script"); err != nil {
		return fmt.Errorf("couldn't get startup-script from metadata, %v", err)
	}

	return nil
}

// TestStartupScriptFailedWindows tests that a script failed execute doesn't crash the vm.
func testStartupScriptFailedWindows(t *testing.T) error {
	if _, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "windows-startup-script-ps1"); err != nil {
		return fmt.Errorf("couldn't get windows-startup-script-ps1 from metadata, %v", err)
	}

	return nil
}

// TestDaemonScriptLinux tests that daemon process started by startup script is still
// running in the VM after execution of startup script
func testDaemonScriptLinux() error {
	daemonOutputPath := "/var/daemon_out.txt"
	bytes, err := ioutil.ReadFile(daemonOutputPath)
	if err != nil {
		return fmt.Errorf("failed to read daemon script PID file: %v", err)
	}
	pid := strings.TrimSpace(string(bytes))
	cmd := exec.Command("ps", "-p", pid)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("Daemon process not running: command \"ps -p %s\" failed: %v, output was: %s", pid, err, output)
	}

	return nil
}

// TestDaemonScriptWindows tests that background cmd process started by startup script is still
// running in the VM after execution of startup script
func testDaemonScriptWindows() error {
	command := `Get-Process cmd`
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		return fmt.Errorf("Daemon process not found: %v", err)
	}

	job := strings.TrimSpace(output.Stdout)
	if !strings.Contains(job, "cmd") {
		return fmt.Errorf("Daemon process not running. Output of Get-Process: %s", job)
	}

	return nil
}

// TestStartupScripts verifies that the standard metadata script could run successfully
// by checking the output content of the Startup script. It also checks that
// the script does not run after a reinstall/upgrade of guest agent.
func TestStartupScripts(t *testing.T) {
	ctx := utils.Context(t)
	testScripts(t, "startup", true)

	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("utils.GetMetadata(ctx, instance, image) = err %v want nil", err)
	}

	// Only perform agent reinstall for non-COS images.
	if !utils.IsCOS(image) {
		reinstallGuestAgent(ctx, t)
		testScripts(t, "startup", false)
	}
}

// Determine if the OS is Windows or Linux and run the appropriate failure test.
func TestStartupScriptsFailed(t *testing.T) {
	if utils.IsWindows() {
		if err := testStartupScriptFailedWindows(t); err != nil {
			t.Fatalf("Startup script failure test failed with error: %v", err)
		}
	} else {
		if err := testStartupScriptFailedLinux(t); err != nil {
			t.Fatalf("Shutdown script failure test failed with error: %v", err)
		}
	}
}

// Determine if the OS is Windows or Linux and run the appropriate daemon test.
func TestDaemonScripts(t *testing.T) {
	if utils.IsWindows() {
		if err := testDaemonScriptWindows(); err != nil {
			t.Fatalf("Daemon script test failed with error: %v", err)
		}
	} else {
		if err := testDaemonScriptLinux(); err != nil {
			t.Fatalf("Daemon script test failed with error: %v", err)
		}
	}
}

// resetStartupScriptURL resets the startup script URL to a non-existent file.
//
// This is used to test the secondary script runner.
func resetStartupScriptURL(t *testing.T, url string) {
	t.Helper()

	instanceName, err := utils.GetInstanceName(utils.Context(t))
	if err != nil {
		t.Fatalf("Failed to get ping VM name: %v", err)
	}

	metadata := utils.GetInstanceMetadata(t, instanceName)
	for _, item := range metadata.Items {
		if item.Key == "startup-script-url" {
			item.Value = &url
		}
	}

	utils.SetInstanceMetadata(t, instanceName, metadata)
}

// getStartupScriptURL returns the startup script URL from the metadata.
//
// The startup script URL is the value of the "startup-script-url" metadata key
// for the given VM name.
func getStartupScriptURL(t *testing.T, name string) string {
	t.Helper()

	data, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "startup-script-url")
	if err != nil {
		t.Fatalf("couldn't get startup-script-url from metadata, %v", err)
	}

	return data
}

// triggerSecondaryScriptRunner triggers the secondary script runner to execute
// the startup script.
//
// This is used to test the secondary script runner.
func triggerSecondaryScriptRunner(t *testing.T) {
	t.Helper()
	t.Logf("Running command...")

	cmd := exec.Command("/usr/bin/google_metadata_script_runner", "startup")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to trigger secondary script runner: %v, output was: %s", err, output)
	}

	t.Logf("Output of secondary script runner: %s", string(output))
}

// matchURLPattern matches the startup script URL against all supported URL
// patterns.
//
// If the URL matches a pattern, the pattern and bucket and object are returned.
func matchURLPattern(t *testing.T, universeDomain, url string) (*scriptData, error) {
	t.Helper()

	bucketRegex := "([a-z0-9][-_.a-z0-9]*)"
	objectRegex := "(.+)"
	domainRegex := regexp.QuoteMeta(universeDomain)

	expressions := map[string]*regexp.Regexp{
		gsPattern:                        regexp.MustCompile(fmt.Sprintf(`^gs://%s/%s$`, bucketRegex, objectRegex)),
		httpStoragePattern:               regexp.MustCompile(fmt.Sprintf(`^http[s]?://%s\.storage\.%s/%s$`, bucketRegex, domainRegex, objectRegex)),
		httpStorageCloudPattern:          regexp.MustCompile(fmt.Sprintf(`^http[s]?://storage\.cloud\.%s/%s/%s$`, domainRegex, bucketRegex, objectRegex)),
		httpStorageCommondataPattern:     regexp.MustCompile(fmt.Sprintf(`^http[s]?://(?:commondata)?storage\.%s/%s/%s$`, domainRegex, bucketRegex, objectRegex)),
		httpStorageCloudGoogleComPattern: regexp.MustCompile(fmt.Sprintf(`^http[s]?://storage\.cloud\.%s/%s/%s$`, domainRegex, bucketRegex, objectRegex)),
	}

	startupScriptURL := getStartupScriptURL(t, "startup-script-url")

	for key, value := range expressions {
		match := value.FindStringSubmatch(startupScriptURL)
		if len(match) == 3 {
			return &scriptData{pattern: key, bucket: match[1], object: match[2]}, nil
		}
	}

	return nil, fmt.Errorf("failed to match startup script URL pattern: %q", startupScriptURL)
}

// writeStatusFile writes a status file to the given temporary directory.
//
// The status file name is the startup script URL pattern, and the file contents
// is "OK".
func writeStatusFile(t *testing.T, universeDomain, startupScriptURL, tmpDir string) error {
	scriptData, err := matchURLPattern(t, universeDomain, startupScriptURL)
	if err != nil {
		return fmt.Errorf("Failed to match startup script URL pattern: %v", err)
	}

	filePath := path.Join(tmpDir, scriptData.pattern)

	if err := ioutil.WriteFile(filePath, []byte("OK"), 0755); err != nil {
		return fmt.Errorf("Failed to write startup script URL to file: %v", err)
	}

	return nil
}

// TestCustomURLPatterns tests the startup script URL patterns.
//
// The startup script URL patterns are:
//
//   - gs://<bucket>/<object>
//   - http://<bucket>.storage.googleapis.com/<object>
//   - http://<bucket>.storage.cloud.googleapis.com/object
//   - http://commondatastorage.googleapis.com/<bucket>/<object>
//   - http://storage.cloud.google.com/<bucket>/<object>
//
// For each pattern, the startup script URL is reset to the pattern, and the
// secondary script runner is triggered. The secondary script runner will write
// a status file if the URL matches the pattern.
func TestCustomURLPatterns(t *testing.T) {
	if utils.IsWindows() {
		t.Skipf("Windows does not support use startup-script-url metadata key.")
	}

	universeDomain, err := utils.GetMetadata(utils.Context(t), "universe", "universe-domain")
	if err != nil {
		universeDomain = "googleapis.com"
	}

	tmpDir := path.Join(os.TempDir(), "test-data")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	startupScriptURL := getStartupScriptURL(t, "startup-script-url")

	// CIT + Daisy will always use gs:// URLS for startup scripts, that signals
	// we are running the main script runner execution, from here on the main
	// execution will orchestrate the startup-script-url changes and secondary
	// runner executions.
	if strings.HasPrefix(startupScriptURL, "gs://") {
		t.Logf("Running main execution...")

		// Iterate through all supported URL patterns, reset the startup-script-url
		// metadata key, and trigger the secondary script runner. The secondary
		// runner will execute the same program but will only write the status file
		// if the URL matches the pattern.
		patterns := map[string]string{
			httpStoragePattern:           fmt.Sprintf("http://<bucket>.storage.%s/<object>", universeDomain),
			httpStorageCloudPattern:      fmt.Sprintf("http://storage.cloud.%s/<bucket>/<object>", universeDomain),
			httpStorageCommondataPattern: fmt.Sprintf("http://commondatastorage.%s/<bucket>/<object>", universeDomain),
		}

		for pattern, url := range patterns {
			t.Logf("Testing pattern: %q", pattern)

			scriptData, err := matchURLPattern(t, universeDomain, url)
			if err != nil {
				t.Fatalf("Failed to match startup script URL pattern: %v", err)
			}

			url := strings.ReplaceAll(url, "<bucket>", scriptData.bucket)
			url = strings.ReplaceAll(url, "<object>", scriptData.object)

			t.Logf("Starting test with URL: %q", url)

			resetStartupScriptURL(t, url)
			triggerSecondaryScriptRunner(t)
		}

		// Check that the status files were created.
		t.Logf("Checking status files in: %q", tmpDir)
		var missingFiles []string

		for pattern := range patterns {
			filePath := path.Join(tmpDir, pattern)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				missingFiles = append(missingFiles, pattern)
				continue
			}

			t.Logf("Status file exists: %v", filePath)
		}

		if len(missingFiles) == 0 {
			return
		}

		t.Fatalf("Missing status files: %v", strings.Join(missingFiles, ", "))
	} else {
		t.Logf("Running secondary execution...")

		// This step is the secondary script runner execution. In here the test will
		// only write the status file assuming the script was executed correctly
		// using the custom URL pattern set by the main execution.
		if err := writeStatusFile(t, universeDomain, startupScriptURL, tmpDir); err != nil {
			t.Fatalf("Failed to write status file: %v", err)
		}
	}
}
