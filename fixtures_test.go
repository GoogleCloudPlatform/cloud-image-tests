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

package imagetest

import (
	"slices"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// TestAddMetadata tests that *TestVM.AddMetadata succeeds and that it
// populates the instance.Metadata map.
func TestAddMetadata(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	tvm.AddMetadata("key", "val")
	if tvm.instance.Metadata == nil {
		t.Errorf("failed to set VM metadata")
	}
	if val, ok := tvm.instance.Metadata["key"]; !ok || val != "val" {
		t.Errorf("invalid metadata set")
	}
	tvm.AddMetadata("key", "val2")
	if val, ok := tvm.instance.Metadata["key"]; !ok || val != "val2" {
		t.Errorf("invalid metadata set")
	}
	tvmb, err := twf.CreateTestVMBeta("vmBeta")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	tvmb.AddMetadata("key", "val")
	if tvmb.instancebeta.Metadata == nil {
		t.Errorf("failed to set VM metadata")
	}
	if val, ok := tvmb.instancebeta.Metadata["key"]; !ok || val != "val" {
		t.Errorf("invalid metadata set")
	}
	tvmb.AddMetadata("key", "val2")
	if val, ok := tvmb.instancebeta.Metadata["key"]; !ok || val != "val2" {
		t.Errorf("invalid metadata set")
	}
}

// TestReboot tests that *TestVM.Reboot succeeds and that the appropriate stop
// and new final wait steps are created in the workflow.
func TestReboot(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	if twf.counter != 0 {
		t.Errorf("step counter not starting at 0")
	}
	if err := tvm.Reboot(); err != nil {
		t.Errorf("failed to reboot: %v", err)
	}
	if twf.counter != 1 {
		t.Errorf("step counter not incremented")
	}
	if _, ok := twf.wf.Steps["stop-vm-1"]; !ok {
		t.Errorf("wait-vm-1 step missing")
	}
	lastStep, err := twf.getLastStepForVM("vm")
	if err != nil {
		t.Errorf("failed to get last step for vm: %v", err)
	}
	if lastStep.WaitForInstancesSignal == nil {
		t.Error("not wait step")
	}
	if step, ok := twf.wf.Steps["wait-started-vm-1"]; !ok || step != lastStep {
		t.Error("not wait-started-vm-1 step")
	}
}

func TestCreateDerivativeVM(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	if twf.counter != 0 {
		t.Errorf("step counter not starting at 0")
	}

	dvm, err := tvm.CreateDerivativeVM("vm")
	if err != nil {
		t.Errorf("failed to create derivative vm: %v", err)
	}
	if dvm.name != "derivative-vm" {
		t.Errorf("unexpected derivative vm name: got %s, want %s", dvm.name, "derivative-vm")
	}
	if twf.counter != 1 {
		t.Errorf("step counter not incremented")
	}
	if _, ok := twf.wf.Steps["create-vms-1"]; !ok {
		t.Errorf("create-vms-1 step missing")
	} else {
		deps := twf.wf.Dependencies["create-vms-1"]
		if !slices.Contains(deps, "detach-disk-vm-1") {
			t.Errorf("create-vms-1 has deps %v, want a dependency on detach-disk-vm-1", deps)
		}
	}

	lastStep, err := twf.getLastStepForVM("derivative-vm")
	if err != nil {
		t.Errorf("failed to get last step for derivative_vm: %v", err)
	}
	if lastStep.WaitForInstancesSignal == nil {
		t.Error("last step for derivative_vm is not WaitForInstancesSignal")
	}
}

func TestResume(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	if twf.counter != 0 {
		t.Errorf("step counter not starting at 0")
	}
	if err := tvm.Resume(); err != nil {
		t.Errorf("failed to resume: %v", err)
	}
	if twf.counter != 1 {
		t.Errorf("step counter not incremented")
	}
	if _, ok := twf.wf.Steps["wait-suspended-vm-1"]; !ok {
		t.Errorf("wait-suspended-vm-1 step missing")
	} else {
		deps := twf.wf.Dependencies["wait-suspended-vm-1"]
		if !slices.Contains(deps, createVMsStepName) {
			t.Errorf("wait-suspended has deps %v, want a dependency on %s", deps, createVMsStepName)
		}
	}
	if _, ok := twf.wf.Steps["resume-vm-1"]; !ok {
		t.Errorf("resume-vm-1 step missing")
	} else {
		deps := twf.wf.Dependencies["resume-vm-1"]
		if !slices.Contains(deps, "wait-suspended-vm-1") {
			t.Errorf("resume has deps %v, want a dependency on wait-suspended-vm-1", deps)
		}
	}
	lastStep, err := twf.getLastStepForVM("vm")
	if err != nil {
		t.Errorf("failed to get last step for vm: %v", err)
	}
	if lastStep.WaitForInstancesSignal == nil {
		t.Error("last step for vm is not WaitForInstancesSignal")
	}
}

func TestCreateVMFromInstanceBeta(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	disks := []*compute.Disk{{Name: "vm"}, {Name: "mountdisk", Type: PdSsd, SizeGb: 100}}
	inst := &daisy.InstanceBeta{}
	inst.Name = "vm"
	tvm, err := twf.CreateTestVMFromInstanceBeta(inst, disks)
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	// once found, expect createInstancesStep.CreateInstances != nil
	// once found, expect createDisksStep.CreateDisks != nil
	var createInstancesStep, createDisksStep *daisy.Step
	for _, step := range twf.wf.Steps {
		// there should only be one create instance step
		if step.CreateInstances != nil {
			if createInstancesStep == nil {
				createInstancesStep = step
			} else {
				t.Errorf("workflow has multiple create instance steps when it should not")
			}
		}

		if step.CreateDisks != nil {
			if createDisksStep == nil {
				createDisksStep = step
			} else {
				t.Errorf("workflow has multiple create disk steps when it should not")
			}
		}
	}

	if createInstancesStep == nil || createInstancesStep.CreateInstances == nil {
		t.Errorf("failed to find create instances step when creating multiple disks")
	}

	if createDisksStep == nil || createDisksStep.CreateDisks == nil {
		t.Errorf("failed to find create disks step when creating multiple disks")
	}

	daisyStepDisksSlice := *(createDisksStep.CreateDisks)
	if len(disks) != len(daisyStepDisksSlice) {
		t.Errorf("found incorrect number of disks in create disk step: expected %d, got %d",
			len(disks), len(daisyStepDisksSlice))
	}

	if twf.counter != 0 {
		t.Errorf("step counter not starting at 0")
	}
	if err := tvm.Reboot(); err != nil {
		t.Errorf("failed to reboot: %v", err)
	}
	if twf.counter != 1 {
		t.Errorf("step counter not incremented")
	}
	if _, ok := twf.wf.Steps["stop-vm-1"]; !ok {
		t.Errorf("wait-vm-1 step missing")
	}
	lastStep, err := twf.getLastStepForVM("vm")
	if err != nil {
		t.Errorf("failed to get last step for vm: %v", err)
	}
	if lastStep.WaitForInstancesSignal == nil {
		t.Error("not wait step")
	}
	waitForInstancesSignalSlice := (*lastStep.WaitForInstancesSignal)
	if len(waitForInstancesSignalSlice) == 0 {
		t.Error("waitForInstancesSignal has no elements in slice")
	}
	waitGuestAttribute := waitForInstancesSignalSlice[0].GuestAttribute
	if waitGuestAttribute == nil {
		t.Error("could not find guest attribute wait step")
	}
	gaNameSpace, gaKeyName := waitGuestAttribute.Namespace, waitGuestAttribute.KeyName
	if gaNameSpace != utils.GuestAttributeTestNamespace || gaKeyName != utils.GuestAttributeTestKey {
		t.Errorf("wrong guest attribute: got namespace, keyname as %s, %s but expected %s, %s", gaNameSpace, gaKeyName, utils.GuestAttributeTestNamespace, utils.GuestAttributeTestKey)
	}
	if step, ok := twf.wf.Steps["wait-started-vm-1"]; !ok || step != lastStep {
		t.Error("not wait-started-vm-1 step")
	}
}

// TestCreateVMMultipleDisks tests that after creating a VM with multiple disks,
// the correct step dependencies are in place.
func TestCreateVMMultipleDisks(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	disks := []*compute.Disk{{Name: "vm"}, {Name: "mountdisk", Type: PdSsd, SizeGb: 100}}
	tvm, err := twf.CreateTestVMMultipleDisks(disks, nil)
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	// once found, expect createInstancesStep.CreateInstances != nil
	// once found, expect createDisksStep.CreateDisks != nil
	var createInstancesStep, createDisksStep *daisy.Step
	for _, step := range twf.wf.Steps {
		// there should only be one create instance step
		if step.CreateInstances != nil {
			if createInstancesStep == nil {
				createInstancesStep = step
			} else {
				t.Errorf("workflow has multiple create instance steps when it should not")
			}
		}

		if step.CreateDisks != nil {
			if createDisksStep == nil {
				createDisksStep = step
			} else {
				t.Errorf("workflow has multiple create disk steps when it should not")
			}
		}
	}

	if createInstancesStep == nil || createInstancesStep.CreateInstances == nil {
		t.Errorf("failed to find create instances step when creating multiple disks")
	}

	if createDisksStep == nil || createDisksStep.CreateDisks == nil {
		t.Errorf("failed to find create disks step when creating multiple disks")
	}

	daisyStepDisksSlice := *(createDisksStep.CreateDisks)
	if len(disks) != len(daisyStepDisksSlice) {
		t.Errorf("found incorrect number of disks in create disk step: expected %d, got %d",
			len(disks), len(daisyStepDisksSlice))
	}

	if twf.counter != 0 {
		t.Errorf("step counter not starting at 0")
	}
	if err := tvm.Reboot(); err != nil {
		t.Errorf("failed to reboot: %v", err)
	}
	if twf.counter != 1 {
		t.Errorf("step counter not incremented")
	}
	if _, ok := twf.wf.Steps["stop-vm-1"]; !ok {
		t.Errorf("wait-vm-1 step missing")
	}
	lastStep, err := twf.getLastStepForVM("vm")
	if err != nil {
		t.Errorf("failed to get last step for vm: %v", err)
	}
	if lastStep.WaitForInstancesSignal == nil {
		t.Error("not wait step")
	}
	waitForInstancesSignalSlice := (*lastStep.WaitForInstancesSignal)
	if len(waitForInstancesSignalSlice) == 0 {
		t.Error("waitForInstancesSignal has no elements in slice")
	}
	waitGuestAttribute := waitForInstancesSignalSlice[0].GuestAttribute
	if waitGuestAttribute == nil {
		t.Error("could not find guest attribute wait step")
	}
	gaNameSpace, gaKeyName := waitGuestAttribute.Namespace, waitGuestAttribute.KeyName
	if gaNameSpace != utils.GuestAttributeTestNamespace || gaKeyName != utils.GuestAttributeTestKey {
		t.Errorf("wrong guest attribute: got namespace, keyname as %s, %s but expected %s, %s", gaNameSpace, gaKeyName, utils.GuestAttributeTestNamespace, utils.GuestAttributeTestKey)
	}
	if step, ok := twf.wf.Steps["wait-started-vm-1"]; !ok || step != lastStep {
		t.Error("not wait-started-vm-1 step")
	}
}

// TestCreateVMRebootGA tests that after creating a VM with multiple disks, if the vm
// is expected to reboot during the test, a special guest attribute is used as the wait signal.
func TestCreateVMRebootGA(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	disks := []*compute.Disk{{Name: "vm"}, {Name: "mountdisk", Type: PdSsd, SizeGb: 100}}
	rebootInst := &daisy.Instance{}
	rebootInst.Metadata = map[string]string{ShouldRebootDuringTest: "true"}
	tvm, err := twf.CreateTestVMMultipleDisks(disks, rebootInst)
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	// once found, expect createInstancesStep.CreateInstances != nil
	// once found, expect createDisksStep.CreateDisks != nil
	var createInstancesStep, createDisksStep *daisy.Step
	for _, step := range twf.wf.Steps {
		// there should only be one create instance step
		if step.CreateInstances != nil {
			if createInstancesStep == nil {
				createInstancesStep = step
			} else {
				t.Errorf("workflow has multiple create instance steps when it should not")
			}
		}

		if step.CreateDisks != nil {
			if createDisksStep == nil {
				createDisksStep = step
			} else {
				t.Errorf("workflow has multiple create disk steps when it should not")
			}
		}
	}

	if createInstancesStep == nil || createInstancesStep.CreateInstances == nil {
		t.Errorf("failed to find create instances step when creating multiple disks")
	}

	if createDisksStep == nil || createDisksStep.CreateDisks == nil {
		t.Errorf("failed to find create disks step when creating multiple disks")
	}

	daisyStepDisksSlice := *(createDisksStep.CreateDisks)
	if len(disks) != len(daisyStepDisksSlice) {
		t.Errorf("found incorrect number of disks in create disk step: expected %d, got %d",
			len(disks), len(daisyStepDisksSlice))
	}

	if twf.counter != 0 {
		t.Errorf("step counter not starting at 0")
	}
	// check for wait step before reboot
	lastStepBeforeReboot, err := twf.getLastStepForVM("vm")
	if err != nil {
		t.Errorf("failed to get last step for vm: %v", err)
	}
	if lastStepBeforeReboot.WaitForInstancesSignal == nil {
		t.Error("not wait step")
	}
	waitForInstancesSignalSlice := (*lastStepBeforeReboot.WaitForInstancesSignal)
	if len(waitForInstancesSignalSlice) == 0 {
		t.Error("waitForInstancesSignal has no elements in slice")
	}
	waitGuestAttribute := waitForInstancesSignalSlice[0].GuestAttribute
	if waitGuestAttribute == nil {
		t.Error("could not find guest attribute wait step")
	}
	gaNameSpace, gaKeyName := waitGuestAttribute.Namespace, waitGuestAttribute.KeyName
	if gaNameSpace != utils.GuestAttributeTestNamespace || gaKeyName != utils.FirstBootGAKey {
		t.Errorf("wrong guest attribute: got namespace, keyname as %s, %s but expected %s, %s", gaNameSpace, gaKeyName, utils.GuestAttributeTestNamespace, utils.FirstBootGAKey)
	}
	if err := tvm.Reboot(); err != nil {
		t.Errorf("failed to reboot: %v", err)
	}
	if twf.counter != 1 {
		t.Errorf("step counter not incremented")
	}
	if _, ok := twf.wf.Steps["stop-vm-1"]; !ok {
		t.Errorf("wait-vm-1 step missing")
	}
	// check for wait step after reboot
	lastStepAfterReboot, err := twf.getLastStepForVM("vm")
	if err != nil {
		t.Errorf("failed to get last step for vm: %v", err)
	}
	if lastStepAfterReboot.WaitForInstancesSignal == nil {
		t.Error("not wait step")
	}
	waitForInstancesSignalSlice = (*lastStepAfterReboot.WaitForInstancesSignal)
	if len(waitForInstancesSignalSlice) == 0 {
		t.Error("waitForInstancesSignal has no elements in slice")
	}
	waitGuestAttribute = waitForInstancesSignalSlice[0].GuestAttribute
	if waitGuestAttribute == nil {
		t.Error("could not find guest attribute wait step")
	}
	gaNameSpace, gaKeyName = waitGuestAttribute.Namespace, waitGuestAttribute.KeyName
	if gaNameSpace != utils.GuestAttributeTestNamespace || gaKeyName != utils.GuestAttributeTestKey {
		t.Errorf("wrong guest attribute: got namespace, keyname as %s, %s but expected %s, %s", gaNameSpace, gaKeyName, utils.GuestAttributeTestNamespace, utils.GuestAttributeTestKey)
	}
	if step, ok := twf.wf.Steps["wait-started-vm-1"]; !ok || step != lastStepAfterReboot {
		t.Error("not wait-started-vm-1 step")
	}
}

// TestRebootMultipleDisks creates a VM using multiple disks, and then runs
// the same tests as TestReboot.
func TestRebootMultipleDisks(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	disks := []*compute.Disk{{Name: "vm"}, {Name: "mountdisk", Type: PdBalanced, SizeGb: 100}}
	testMachineType := "c3-standard-4"
	pdInst := &daisy.Instance{}
	pdInst.MachineType = testMachineType
	tvm, err := twf.CreateTestVMMultipleDisks(disks, pdInst)
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	if tvm.instance.MachineType != testMachineType {
		t.Errorf("failed to set test machine type, expected %s but got %s", testMachineType, tvm.instance.MachineType)
	}
	if twf.counter != 0 {
		t.Errorf("step counter not starting at 0")
	}
	if err := tvm.Reboot(); err != nil {
		t.Errorf("failed to reboot: %v", err)
	}
	if twf.counter != 1 {
		t.Errorf("step counter not incremented")
	}
	if _, ok := twf.wf.Steps["stop-vm-1"]; !ok {
		t.Errorf("wait-vm-1 step missing")
	}
	lastStep, err := twf.getLastStepForVM("vm")
	if err != nil {
		t.Errorf("failed to get last step for vm: %v", err)
	}
	if lastStep.WaitForInstancesSignal == nil {
		t.Error("not wait step")
	}
	if step, ok := twf.wf.Steps["wait-started-vm-1"]; !ok || step != lastStep {
		t.Error("not wait-started-vm-1 step")
	}
}

// TestResizeDiskAndReboot tests that *TestVM.ResizeDiskAndReboot succeeds and
// that the appropriate resize and new final wait steps are created in the
// workflow.
func TestResizeDiskAndReboot(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	if err := tvm.ResizeDiskAndReboot(200); err != nil {
		t.Errorf("failed to reboot: %v", err)
	}
	if _, ok := twf.wf.Steps["resize-disk-vm-1"]; !ok {
		t.Errorf("wait-vm-1 step missing")
	}
	step, err := twf.getLastStepForVM("vm")
	if err != nil {
		t.Errorf("failed to get last step for vm: %v", err)
	}
	if step.WaitForInstancesSignal == nil {
		t.Error("not wait step")
	}
	if twf.wf.Steps["wait-started-vm-2"] != step {
		t.Error("not wait-started-vm-2 step")
	}
	tvmb, err := twf.CreateTestVM("vmbeta")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	if err := tvmb.ResizeDiskAndReboot(200); err != nil {
		t.Errorf("failed to reboot: %v", err)
	}
	if _, ok := twf.wf.Steps["resize-disk-vm-1"]; !ok {
		t.Errorf("wait-vm-1 step missing")
	}
	step, err = twf.getLastStepForVM("vmbeta")
	if err != nil {
		t.Errorf("failed to get last step for vm: %v", err)
	}
	if step.WaitForInstancesSignal == nil {
		t.Error("not wait step")
	}
	if twf.wf.Steps["wait-started-vmbeta-4"] != step {
		t.Error("not wait-started-vmbeta-4")
	}
}

// TestEnableSecureBoot tests that *TestVM.EnableSecureBoot succeeds and
// populates the ShieldedInstanceConfig struct.
func TestEnableSecureBoot(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	tvm.EnableSecureBoot()
	if !tvm.instance.ShieldedInstanceConfig.EnableSecureBoot {
		t.Errorf("test vm does not have secure boot enabled")
	}
	tvmb, err := twf.CreateTestVMBeta("vmbeta")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	tvmb.EnableSecureBoot()
	if !tvmb.instancebeta.ShieldedInstanceConfig.EnableSecureBoot {
		t.Errorf("test vmbeta does not have secure boot enabled")
	}
}

// TestWaitForQuotaStep tests that quotas are successfully appended to the wait
// step.
func TestWaitForQuotaStep(t *testing.T) {
	testcases := []struct {
		name   string
		input  []*daisy.QuotaAvailable
		output []*daisy.QuotaAvailable
	}{
		{
			name:   "single quota",
			input:  []*daisy.QuotaAvailable{{Metric: "test", Units: 1, Region: "us-central1"}},
			output: []*daisy.QuotaAvailable{{Metric: "test", Units: 1, Region: "us-central1"}},
		},
		{
			name:   "two independent quotas",
			input:  []*daisy.QuotaAvailable{{Metric: "test2", Units: 2, Region: "us-central1"}, {Metric: "test1", Units: 1, Region: "us-west1"}},
			output: []*daisy.QuotaAvailable{{Metric: "test2", Units: 2, Region: "us-central1"}, {Metric: "test1", Units: 1, Region: "us-west1"}},
		},
		{
			name:   "two quotas same region",
			input:  []*daisy.QuotaAvailable{{Metric: "test2", Units: 2, Region: "us-central1"}, {Metric: "test1", Units: 1, Region: "us-central1"}},
			output: []*daisy.QuotaAvailable{{Metric: "test2", Units: 2, Region: "us-central1"}, {Metric: "test1", Units: 1, Region: "us-central1"}},
		},
		{
			name:   "two quotas same metric",
			input:  []*daisy.QuotaAvailable{{Metric: "test2", Units: 2, Region: "us-central1"}, {Metric: "test2", Units: 1, Region: "us-west1"}},
			output: []*daisy.QuotaAvailable{{Metric: "test2", Units: 2, Region: "us-central1"}, {Metric: "test2", Units: 1, Region: "us-west1"}},
		},
		{
			name:   "two identical quotas",
			input:  []*daisy.QuotaAvailable{{Metric: "test2", Units: 2, Region: "us-central1"}, {Metric: "test2", Units: 1, Region: "us-central1"}},
			output: []*daisy.QuotaAvailable{{Metric: "test2", Units: 3, Region: "us-central1"}},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			twf := NewTestWorkflowForUnitTest("name", "image", "30m")
			for _, quota := range tc.input {
				err := twf.waitForQuotaStep(quota, tc.name)
				if err != nil {
					t.Errorf("failed to append quota: %v", err)
				}
			}
			quotaStep, ok := twf.wf.Steps[tc.name]
			if !ok {
				t.Errorf("Could not find wait for vm quota step")
			}
			if len(quotaStep.WaitForAvailableQuotas.Quotas) != len(tc.output) {
				t.Errorf("unexpected output length from WaitForVMQuota, got %d want %d", len(quotaStep.WaitForAvailableQuotas.Quotas), len(tc.output))
			}
			for i := range tc.output {
				q := quotaStep.WaitForAvailableQuotas.Quotas[i]
				if q.Metric != tc.output[i].Metric || q.Units != tc.output[i].Units || q.Region != tc.output[i].Region {
					t.Errorf("unexpected quota at position %d\ngot %v\nwant %v", i, *q, *tc.output[i])
				}
			}
		})
	}
}

// TestUseGVNIC tests that *TestVM.UseGVNIC succeeds and
// populates the Network Interface with a NIC type of GVNIC.
func TestUseGVNIC(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	tvm.UseGVNIC()
	if tvm.instance.NetworkInterfaces == nil {
		t.Errorf("VM Network Interfaces is nil")
	}
	if tvm.instance.NetworkInterfaces[0].NicType != "GVNIC" {
		t.Errorf("VM Network Interface type not set to GVNIC")
	}
	tvmb, err := twf.CreateTestVMBeta("vmbeta")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	tvmb.UseGVNIC()
	if tvmb.instancebeta.NetworkInterfaces == nil {
		t.Errorf("VM Network Interfaces is nil")
	}
	if tvmb.instancebeta.NetworkInterfaces[0].NicType != "GVNIC" {
		t.Errorf("VM Network Interface type not set to GVNIC")
	}
}

// TestAddDisk tests that *TestVM.AddDisk succeeds and attaches a custom disk
// to the VM.
func TestAddDisk(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	initParams := &compute.AttachedDiskInitializeParams{
		DiskSizeGb: 375,
		DiskType:   "zones/us-central1-a/diskTypes/local-ssd",
	}
	if err := tvm.AddDisk("SCRATCH", initParams); err != nil {
		t.Errorf("failed to add disk: %v", err)
	}
	if tvm.instance.Disks == nil {
		t.Errorf("VM Disks is nil")
	}

	// There should be 2 disks, the boot disk and one local SSD.
	if len(tvm.instance.Disks) != 2 {
		t.Errorf("VM Disks length is not 2")
	}

	// The local SSDs should be the last two disks.
	if tvm.instance.Disks[1].Type != "SCRATCH" {
		t.Errorf("VM Disk 1 not type SCRATCH")
	}
}

func TestCreateNetworkFromDaisyNetwork(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	net := &daisy.Network{}
	net.Name = "test-network"
	net.IPv4Range = "10.0.0.0/24"
	network, err := twf.CreateNetworkFromDaisyNetwork(net)
	if err != nil {
		t.Errorf("twf.CreateNetworkFromDaisyNetwork(net) = %v want nil", err)
	}
	if network.name != net.Name {
		t.Errorf("network.name != net.Name, got %q want %q", network.name, net.Name)
	}
}

// TestAddAliasIPRanges tests that *TestVM.AddAliasIPRanges succeeds and that
// it fails if *TestVM.AddCustomNetwork hasn't been called first.
func TestAddAliasIPRanges(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	if err := tvm.AddAliasIPRanges("aliasIPRange", "rangeName"); err == nil {
		t.Fatalf("shouldn't be able to set alias IP without calling setcustomnetwork")
	}
	network, err := twf.CreateNetwork("network", true)
	if err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	if err := tvm.AddCustomNetwork(network, nil); err != nil {
		t.Errorf("failed to set custom network: %v", err)
	}
	if err := tvm.AddAliasIPRanges("aliasIPRange", "rangeName"); err != nil {
		t.Fatalf("error adding alias IP range: %v", err)
	}
	if tvm.instance.NetworkInterfaces[0].AliasIpRanges == nil {
		t.Errorf("VM alias IP is nil")
	}
	tvmb, err := twf.CreateTestVMBeta("vmbeta")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	if err := tvmb.AddAliasIPRanges("aliasIPRange", "rangeName"); err == nil {
		t.Fatalf("shouldn't be able to set alias IP without calling setcustomnetwork")
	}
	if err := tvmb.AddCustomNetwork(network, nil); err != nil {
		t.Errorf("failed to set custom network: %v", err)
	}
	if err := tvmb.AddAliasIPRanges("aliasIPRange", "rangeName"); err != nil {
		t.Fatalf("error adding alias IP range: %v", err)
	}
	if tvmb.instancebeta.NetworkInterfaces[0].AliasIpRanges == nil {
		t.Errorf("VM alias IP is nil")
	}
}

// TestSetCustomNetwork tests that *TestVM.AddCustomNetwork succeeds and that
// it fails if testworkflow.CreateNetwork has not been called first.
func TestSetCustomNetwork(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	network, err := twf.CreateNetwork("network", true)
	if err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	if err := tvm.AddCustomNetwork(network, nil); err != nil {
		t.Errorf("failed to set custom network: %v", err)
	}
	tvmb, err := twf.CreateTestVMBeta("vmbeta")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	network, err = twf.CreateNetwork("network", true)
	if err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	if err := tvmb.AddCustomNetwork(network, nil); err != nil {
		t.Errorf("failed to set custom network: %v", err)
	}
}

// TestSetCustomNetworkAndSubnetwork tests that *TestVM.AddCustomNetwork
// succeeds with a subnet argument and that it fails if
// *Network.CreateSubnetwork has not been called first.
func TestSetCustomNetworkAndSubnetwork(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	network, err := twf.CreateNetwork("network", false)
	if err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	if err := tvm.AddCustomNetwork(network, nil); err == nil {
		t.Errorf("should have gotten an error using no subnet with custom mode network.")
	}
	subnet, err := network.CreateSubnetwork("subnet", "ipRange")
	if err != nil {
		t.Errorf("failed to create subnetwork: %v", err)
	}
	if err := tvm.AddCustomNetwork(network, subnet); err != nil {
		t.Errorf("failed to set custom network and subnetwork: %v", err)
	}
}

// TestAddSecondaryRange tests that AddSecondaryRange populates the
// subnet.SecondaryIpRanges struct.
func TestAddSecondaryRange(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	network, err := twf.CreateNetwork("network", false)
	if err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	subnet, err := network.CreateSubnetwork("subnet", "ipRange")
	if err != nil {
		t.Errorf("failed to create subnetwork: %v", err)
	}
	if subnet.subnetwork.SecondaryIpRanges != nil {
		t.Errorf("Subnet didn't have nil secondary ranges at creation")
	}
	subnet.AddSecondaryRange("rangeName", "ipRange")
	if subnet.subnetwork.SecondaryIpRanges == nil {
		t.Errorf("Subnet has nil secondary range")
	}
}

func TestSetRegion(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	network, err := twf.CreateNetwork("network", false)
	if err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	subnet, err := network.CreateSubnetwork("subnet", "ipRange")
	if err != nil {
		t.Errorf("failed to create subnetwork: %v", err)
	}
	subnet.SetRegion("testregion")
	if subnet.subnetwork.Region != "testregion" {
		t.Errorf("Subnet has unexpected region, got %s want testregion", subnet.subnetwork.Region)
	}
}

// TestCreateNetworkDependenciesReverse tests that the create-vms step depends
// on the create-networks step if they are created in order.
func TestCreateNetworkDependencies(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	if _, err := twf.CreateNetwork("network", false); err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	if _, err := twf.CreateTestVM("vm"); err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	if _, ok := twf.wf.Dependencies[createNetworkStepName]; ok {
		t.Errorf("network step has unnecessary dependencies")
	}
	deps, ok := twf.wf.Dependencies[createVMsStepName]
	if !ok {
		t.Errorf("create-vms step missing dependencies")
	}
	var found bool
	for _, dep := range deps {
		if dep == createNetworkStepName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("create-vms step does not depend on create-networks step")
	}
}

// TestCreateNetworkDependenciesReverse tests that the create-vms step depends
// on the create-networks step if they are created in reverse.
func TestCreateNetworkDependenciesReverse(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	if _, err := twf.CreateTestVM("vm"); err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	if _, err := twf.CreateNetwork("network", false); err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	if _, ok := twf.wf.Dependencies[createNetworkStepName]; ok {
		t.Errorf("network step has unnecessary dependencies")
	}
	deps, ok := twf.wf.Dependencies[createVMsStepName]
	if !ok {
		t.Errorf("create-vms step missing dependencies")
	}
	var found bool
	for _, dep := range deps {
		if dep == createNetworkStepName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("create-vms step does not depend on create-networks step")
	}
}

func TestAddUser(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	tvm.AddUser("username", "PUBKEY1")
	if tvm.instance.Metadata == nil {
		t.Fatalf("instance metadata is nil")
	}
	keys, ok := tvm.instance.Metadata["ssh-keys"]
	if !ok {
		t.Fatalf("\"ssh-keys\" key not added to instance")
	}
	if keys != "username:PUBKEY1" {
		t.Fatalf("\"ssh-keys\" key malformed")
	}
	tvm.AddUser("username2", "PUBKEY2")
	if keys, ok := tvm.instance.Metadata["ssh-keys"]; !ok || keys != "username:PUBKEY1\nusername2:PUBKEY2" {
		t.Errorf("\"ssh-keys\" key malformed after repeated entry")
	}
	tvmb, err := twf.CreateTestVMBeta("vmBeta")
	if err != nil {
		t.Errorf("failed to create network: %v", err)
	}
	tvmb.AddUser("username", "PUBKEY1")
	if tvmb.instancebeta.Metadata == nil {
		t.Fatalf("instance metadata is nil")
	}
	keys, ok = tvmb.instancebeta.Metadata["ssh-keys"]
	if !ok {
		t.Fatalf("\"ssh-keys\" key not added to instance")
	}
	if keys != "username:PUBKEY1" {
		t.Fatalf("\"ssh-keys\" key malformed")
	}
	tvmb.AddUser("username2", "PUBKEY2")
	if keys, ok := tvmb.instancebeta.Metadata["ssh-keys"]; !ok || keys != "username:PUBKEY1\nusername2:PUBKEY2" {
		t.Errorf("\"ssh-keys\" key malformed after repeated entry")
	}
}

func TestForceMachineType(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	if tvm.instance.MachineType != "" {
		t.Errorf("machine type already set: %v", tvm.instance.MachineType)
	}
	tvm.ForceMachineType("t2a-standard-1")
	if tvm.instance.MachineType != "t2a-standard-1" {
		t.Errorf("could not set test machine type, got %q, want t2a-standard-1", tvm.instance.MachineType)
	}
}

func TestForceZone(t *testing.T) {
	twf := NewTestWorkflowForUnitTest("name", "image", "30m")
	tvm, err := twf.CreateTestVM("vm")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	if tvm.instance.Zone != "" {
		t.Errorf("machine zone already set: %v", tvm.instance.Zone)
	}
	tvm.ForceZone("us-east1-a")
	if tvm.instance.Zone != "us-east1-a" {
		t.Errorf("could not set test zone, got %q, want us-east1-a", tvm.instance.Zone)
	}
	tvmb, err := twf.CreateTestVMBeta("vmbeta")
	if err != nil {
		t.Errorf("failed to create test vm: %v", err)
	}
	if tvmb.instancebeta.Zone != "" {
		t.Errorf("machine zone already set: %v", tvmb.instancebeta.Zone)
	}
	tvmb.ForceZone("us-east1-a")
	if tvmb.instancebeta.Zone != "us-east1-a" {
		t.Errorf("could not set test zone, got %q, want us-east1-a", tvm.instance.Zone)
	}
}
