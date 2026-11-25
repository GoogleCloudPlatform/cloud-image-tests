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

package pluginmanager

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

const (
	// The first boot writes the marker before it creates the policy and
	// overwrites it with vmExtMarkerInstalled, vmExtMarkerSkipped plus the
	// reason, or the failure message at the end, so the second boot knows
	// what it is verifying.
	linuxVMExtMarkerFile   = "/var/cit-vmext-marker"
	windowsVMExtMarkerFile = `C:\cit-vmext-marker`
	vmExtMarkerStarted     = "started"
	vmExtMarkerInstalled   = "installed"
	vmExtMarkerSkipped     = "skipped: "
	vmExtPolicyPriority    = 1000
	vmExtPollInterval      = 15 * time.Second
	// The VM extension service inspects a VM without extensions every 8
	// minutes, so the install wait must be longer than that.
	vmExtPollTimeout     = 10 * time.Minute
	vmExtRelaunchTimeout = 2 * time.Minute
	// vmExtEventsHint names the log that says why nothing was installed.
	vmExtEventsHint = "The VM's compute.googleapis.com/vm_extension_policies log in Cloud Logging has the reason, for example INCOMPATIBLE or SERVICE_DISABLED."
)

type vmExtTarget struct {
	project    string
	zone       string
	instance   string
	extension  string
	policyName string
	// installDir lacks the revision suffix the plugin manager appends.
	installDir string
	markerFile string
	svc        *compute.Service
}

func newVMExtTarget(ctx context.Context, t *testing.T) *vmExtTarget {
	t.Helper()
	project, zone, err := utils.GetProjectZone(ctx)
	if err != nil {
		t.Fatalf("Failed to get project and zone: %v", err)
	}
	instance, err := utils.GetInstanceName(ctx)
	if err != nil {
		t.Fatalf("Failed to get instance name: %v", err)
	}
	id, err := utils.GetMetadata(ctx, "instance", "id")
	if err != nil {
		t.Fatalf("Failed to get instance id: %v", err)
	}
	svc, err := compute.NewService(ctx)
	if err != nil {
		t.Fatalf("Failed to create compute client: %v", err)
	}
	markerFile := linuxVMExtMarkerFile
	if utils.IsWindows() {
		markerFile = windowsVMExtMarkerFile
	}
	return &vmExtTarget{
		project:    project,
		zone:       zone,
		instance:   instance,
		extension:  vmExtExtension,
		policyName: "cit-vmext-" + id,
		installDir: filepath.Join(agentBaseDir(t), "plugins", vmExtExtension),
		markerFile: markerFile,
		svc:        svc,
	}
}

func (v *vmExtTarget) waitOp(ctx context.Context, op *compute.Operation) error {
	for op.Status != "DONE" {
		var err error
		op, err = v.svc.ZoneOperations.Wait(v.project, v.zone, op.Name).Context(ctx).Do()
		if err != nil {
			return err
		}
	}
	if op.Error != nil {
		var msgs []string
		for _, e := range op.Error.Errors {
			msgs = append(msgs, e.Message)
		}
		return fmt.Errorf("operation %s failed: %s", op.Name, strings.Join(msgs, "; "))
	}
	return nil
}

func (v *vmExtTarget) insertPolicy(ctx context.Context, label string) error {
	policy := &compute.VmExtensionPolicy{
		Name:        v.policyName,
		Description: "CIT pluginmanager TestVMExtensionLifecycle for " + v.instance,
		Priority:    vmExtPolicyPriority,
		ExtensionPolicies: map[string]compute.VmExtensionPolicyExtensionPolicy{
			v.extension: {},
		},
		InstanceSelectors: []*compute.VmExtensionPolicyInstanceSelector{{
			LabelSelector: &compute.VmExtensionPolicyLabelSelector{
				InclusionLabels: map[string]string{vmExtLabelKey: label},
			},
		}},
	}
	op, err := v.svc.ZoneVmExtensionPolicies.Insert(v.project, v.zone, policy).Context(ctx).Do()
	if err != nil {
		return err
	}
	return v.waitOp(ctx, op)
}

func (v *vmExtTarget) deletePolicy(ctx context.Context) error {
	op, err := v.svc.ZoneVmExtensionPolicies.Delete(v.project, v.zone, v.policyName).Context(ctx).Do()
	if httpStatus(err) == http.StatusNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	return v.waitOp(ctx, op)
}

func (v *vmExtTarget) installDirs() []string {
	dirs, _ := filepath.Glob(v.installDir + "_*")
	return dirs
}

// status is shared by the installed and removed poll functions. It reports the
// install dirs and whether the plugin process is running, with a detail string
// for the failure message.
func (v *vmExtTarget) status() (dirs []string, running bool, detail string, err error) {
	dirs = v.installDirs()
	running, err = processWithPathPrefix(v.installDir)
	if err != nil {
		return nil, false, err.Error(), err
	}
	return dirs, running, fmt.Sprintf("install dirs %v, process running %t", dirs, running), nil
}

func (v *vmExtTarget) installed() (bool, string) {
	dirs, running, detail, err := v.status()
	return err == nil && len(dirs) > 0 && running, detail
}

func (v *vmExtTarget) removed() (bool, string) {
	dirs, running, detail, err := v.status()
	return err == nil && len(dirs) == 0 && !running, detail
}

func processWithPathPrefix(prefix string) (bool, error) {
	if utils.IsWindows() {
		cmd := fmt.Sprintf(`@(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -like '%s*' }).Count`, prefix)
		out, err := utils.RunPowershellCmd(cmd)
		if err != nil {
			return false, fmt.Errorf("Get-CimInstance Win32_Process: %v, stderr: %s", err, out.Stderr)
		}
		return strings.TrimSpace(out.Stdout) != "0", nil
	}
	out, err := exec.Command("ps", "-e", "-o", "command").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("ps: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return true, nil
		}
	}
	return false, nil
}

// httpStatus returns the status code of a Google API error, or 0 for any
// other error.
func httpStatus(err error) int {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		return gerr.Code
	}
	return 0
}

// TestVMExtensionLifecycle installs the extension through a zonal VM extension
// policy on the first boot, then checks after the reboot that it is still
// installed and that deleting the policy removes it.
func TestVMExtensionLifecycle(t *testing.T) {
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("Failed to get image from metadata: %v", err)
	}
	if reason := vmExtUnsupportedReason(path.Base(image)); reason != "" {
		t.Skip(reason)
	}
	if _, err := ggactlPluginPath(); err != nil {
		t.Skipf("Image has no plugin manager: %v", err)
	}
	v := newVMExtTarget(ctx, t)
	if firstBoot(ctx) {
		installOnFirstBoot(ctx, t, v)
		return
	}
	verifyAfterReboot(ctx, t, v)
}

// firstBoot reports whether this run is the one before CIT reboots the VM.
// The test wrapper sets the first-boot guest attribute when that run ends.
func firstBoot(ctx context.Context) bool {
	_, err := utils.GetMetadata(ctx, "instance", "guest-attributes", utils.GuestAttributeTestNamespace, utils.FirstBootGAKey)
	return err != nil
}

func writeMarker(t *testing.T, v *vmExtTarget, content string) {
	t.Helper()
	if err := os.WriteFile(v.markerFile, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write marker file: %v", err)
	}
}

// endFirstBoot records the outcome in the marker file so the second boot
// reports it, then fails or skips.
func endFirstBoot(t *testing.T, v *vmExtTarget, msg string, skip bool) {
	t.Helper()
	content := msg
	if skip {
		content = vmExtMarkerSkipped + msg
	}
	if err := os.WriteFile(v.markerFile, []byte(content), 0o644); err != nil {
		t.Logf("Failed to write marker file: %v", err)
	}
	if skip {
		t.Skip(msg)
	}
	t.Fatal(msg)
}

func installOnFirstBoot(ctx context.Context, t *testing.T, v *vmExtTarget) {
	t.Helper()
	label, err := utils.GetMetadata(ctx, "instance", "attributes", vmExtLabelMetadataKey)
	if err != nil {
		t.Fatalf("Failed to get %s metadata: %v", vmExtLabelMetadataKey, err)
	}
	// The policy must outlive this boot when the test passes.
	t.Cleanup(func() {
		if t.Failed() || t.Skipped() {
			if err := v.deletePolicy(context.Background()); err != nil {
				t.Logf("cleanup: delete policy %s: %v", v.policyName, err)
			}
		}
	})
	writeMarker(t, v, vmExtMarkerStarted)
	if err := v.insertPolicy(ctx, label); err != nil {
		if httpStatus(err) == http.StatusForbidden {
			endFirstBoot(t, v, fmt.Sprintf("Insert policy %s denied, grant compute.vmExtensionPolicies.* to the VM service account: %v", v.policyName, err), true)
		}
		endFirstBoot(t, v, fmt.Sprintf("Failed to insert policy %s: %v", v.policyName, err), false)
	}
	t.Logf("Policy %s created for %s with selector %s=%s", v.policyName, v.extension, vmExtLabelKey, label)
	start := time.Now()
	if err := utils.PollUntil(ctx, v.extension+" install", vmExtPollInterval, vmExtPollTimeout, v.installed); err != nil {
		endFirstBoot(t, v, fmt.Sprintf("%s was not installed: %v. %s", v.extension, err, vmExtEventsHint), false)
	}
	t.Logf("%s installed after %s", v.extension, time.Since(start).Round(time.Second))
	writeMarker(t, v, vmExtMarkerInstalled)
}

func verifyAfterReboot(ctx context.Context, t *testing.T, v *vmExtTarget) {
	t.Helper()
	t.Cleanup(func() {
		if err := v.deletePolicy(context.Background()); err != nil {
			t.Logf("cleanup: delete policy %s: %v", v.policyName, err)
		}
		os.Remove(v.markerFile)
	})
	marker, err := os.ReadFile(v.markerFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		t.Fatalf("First boot did not run the %s install", v.extension)
	case err != nil:
		t.Fatalf("Failed to read marker file: %v", err)
	case string(marker) == vmExtMarkerStarted:
		t.Fatalf("First boot did not finish the %s install", v.extension)
	case strings.HasPrefix(string(marker), vmExtMarkerSkipped):
		t.Skip(strings.TrimPrefix(string(marker), vmExtMarkerSkipped))
	case string(marker) != vmExtMarkerInstalled:
		t.Fatalf("First boot: %s", marker)
	}
	if len(v.installDirs()) == 0 {
		t.Fatalf("%s install dir %s_* did not survive the reboot", v.extension, v.installDir)
	}
	// The manager relaunches installed plugins asynchronously after boot.
	if err := utils.PollUntil(ctx, v.extension+" start after reboot", vmExtPollInterval, vmExtRelaunchTimeout, v.installed); err != nil {
		t.Fatalf("%s did not start after the reboot: %v", v.extension, err)
	}
	t.Logf("%s still installed after reboot", v.extension)
	if hybrid, _ := utils.GetMetadata(ctx, "instance", "attributes", vmExtHybridMetadataKey); hybrid == "true" {
		verifyHybridUnits(t)
	}
	start := time.Now()
	if err := v.deletePolicy(ctx); err != nil {
		t.Fatalf("Failed to delete policy %s: %v", v.policyName, err)
	}
	if err := utils.PollUntil(ctx, v.extension+" removal", vmExtPollInterval, vmExtPollTimeout, v.removed); err != nil {
		t.Fatalf("%s was not removed after the policy was deleted: %v. %s", v.extension, err, vmExtEventsHint)
	}
	t.Logf("%s removed after %s", v.extension, time.Since(start).Round(time.Second))
}
