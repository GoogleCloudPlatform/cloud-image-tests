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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	betacompute "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

const (
	// vmExtMarkerFile is written at the end of the first boot's run so the
	// second boot's run knows the policy already exists.
	vmExtMarkerFile     = "/var/cit-vmext-marker"
	vmExtPolicyPriority = 1000
	vmExtPollInterval   = 15 * time.Second
	vmExtPollTimeout    = 12 * time.Minute
)

type vmExtTarget struct {
	project    string
	zone       string
	instance   string
	extension  string
	policyName string
	// installDir lacks the revision suffix the plugin manager appends.
	installDir string
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
	return &vmExtTarget{
		project:    project,
		zone:       zone,
		instance:   instance,
		extension:  vmExtExtension,
		policyName: "cit-vmext-" + id,
		installDir: filepath.Join(linuxBaseDir, id, "plugins", vmExtExtension),
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

// installed is a poll function: it reports whether the extension is installed
// and running, with a detail string for the failure message.
func (v *vmExtTarget) installed() (bool, string) {
	dirs, _ := filepath.Glob(v.installDir + "_*")
	running, err := processWithPathPrefix(v.installDir)
	if err != nil {
		return false, err.Error()
	}
	detail := fmt.Sprintf("install dirs %v, process running %t", dirs, running)
	return len(dirs) > 0 && running, detail
}

func (v *vmExtTarget) removed() (bool, string) {
	ok, detail := v.installed()
	return !ok, detail
}

func processWithPathPrefix(prefix string) (bool, error) {
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

// logExtensionStates is informational. The beta state API is not enabled for
// every project, so its result never fails the test.
func (v *vmExtTarget) logExtensionStates(ctx context.Context, t *testing.T) {
	t.Helper()
	svc, err := betacompute.NewService(ctx)
	if err != nil {
		t.Logf("beta compute client: %v", err)
		return
	}
	resp, err := svc.Instances.ListVmExtensionStates(v.project, v.zone, v.instance).Context(ctx).Do()
	if err != nil {
		t.Logf("instances.listVmExtensionStates: %v", err)
		return
	}
	if len(resp.Items) == 0 {
		t.Logf("instances.listVmExtensionStates reported no extensions")
	}
	for _, st := range resp.Items {
		t.Logf("extension state: %s version %q enforcement %s health %s policy %s", st.Name, st.Version, st.EnforcementState, st.HealthStatus, st.PolicyId)
	}
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
	utils.LinuxOnly(t)
	ctx := utils.Context(t)
	v := newVMExtTarget(ctx, t)
	if _, err := os.Stat(vmExtMarkerFile); os.IsNotExist(err) {
		installOnFirstBoot(ctx, t, v)
		return
	}
	verifyAfterReboot(ctx, t, v)
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
	if err := v.insertPolicy(ctx, label); err != nil {
		if httpStatus(err) == http.StatusForbidden {
			t.Skipf("Insert policy %s denied, grant compute.vmExtensionPolicies.* to the VM service account: %v", v.policyName, err)
		}
		t.Fatalf("Failed to insert policy %s: %v", v.policyName, err)
	}
	t.Logf("Policy %s created for %s with selector %s=%s", v.policyName, v.extension, vmExtLabelKey, label)
	start := time.Now()
	if err := utils.PollUntil(ctx, v.extension+" install", vmExtPollInterval, vmExtPollTimeout, v.installed); err != nil {
		t.Fatalf("%s was not installed: %v", v.extension, err)
	}
	t.Logf("%s installed after %s", v.extension, time.Since(start).Round(time.Second))
	v.logExtensionStates(ctx, t)
	if err := os.WriteFile(vmExtMarkerFile, []byte(v.policyName), 0o644); err != nil {
		t.Fatalf("Failed to write marker file: %v", err)
	}
}

func verifyAfterReboot(ctx context.Context, t *testing.T, v *vmExtTarget) {
	t.Helper()
	t.Cleanup(func() {
		if err := v.deletePolicy(context.Background()); err != nil {
			t.Logf("cleanup: delete policy %s: %v", v.policyName, err)
		}
		os.Remove(vmExtMarkerFile)
	})
	if ok, detail := v.installed(); !ok {
		t.Fatalf("%s did not survive the reboot: %s", v.extension, detail)
	}
	t.Logf("%s still installed after reboot", v.extension)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("Failed to get image from metadata: %v", err)
	}
	if utils.IsUbuntu(image) {
		verifyHybridUnits(t)
	}
	start := time.Now()
	if err := v.deletePolicy(ctx); err != nil {
		t.Fatalf("Failed to delete policy %s: %v", v.policyName, err)
	}
	if err := utils.PollUntil(ctx, v.extension+" removal", vmExtPollInterval, vmExtPollTimeout, v.removed); err != nil {
		t.Fatalf("%s was not removed after the policy was deleted: %v", v.extension, err)
	}
	t.Logf("%s removed after %s", v.extension, time.Since(start).Round(time.Second))
	v.logExtensionStates(ctx, t)
}
