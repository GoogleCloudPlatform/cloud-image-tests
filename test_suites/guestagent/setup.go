// Copyright 2023 Google LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     https://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package guestagent is a CIT suite for testing guest agent features.
package guestagent

import (
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
const (
	Name        = "guestagent"
	mwlidVMName = "mwlid" // Name of the VM used for mwlid tests.
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	diskType := imagetest.DiskTypeNeeded(t.MachineType.Name)

	telemetrydisabledinst := &daisy.Instance{}
	telemetrydisabledinst.Scopes = []string{"https://www.googleapis.com/auth/cloud-platform"}
	telemetrydisabledinst.Name = "telemetryDisabled"
	telemetrydisabledvm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: telemetrydisabledinst.Name, Type: diskType}}, telemetrydisabledinst)
	if err != nil {
		return err
	}
	telemetrydisabledvm.AddMetadata("disable-guest-telemetry", "true")
	telemetrydisabledvm.RunTests("TestTelemetryDisabled")

	telemetryenabledinst := &daisy.Instance{}
	telemetryenabledinst.Scopes = []string{"https://www.googleapis.com/auth/cloud-platform"}
	telemetryenabledinst.Name = "telemetryEnabled"
	telemetryenabledvm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: telemetryenabledinst.Name, Type: diskType}}, telemetryenabledinst)
	if err != nil {
		return err
	}
	telemetryenabledvm.AddMetadata("disable-guest-telemetry", "false")
	telemetryenabledvm.RunTests("TestTelemetryEnabled|TestServiceConfig")

	// Skip the snapshot tests on Windows client images, and any images using hyperdisk.
	if !utils.IsWindowsClient(t.Image.Name) && !strings.Contains(diskType, "hyperdisk") {
		snapshotinst := &daisy.Instance{}
		snapshotinst.Scopes = []string{"https://www.googleapis.com/auth/cloud-platform"}
		snapshotinst.Name = "snapshotScripts"
		snapshotvm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: snapshotinst.Name, Type: diskType}}, snapshotinst)
		if err != nil {
			return err
		}
		snapshotvm.RunTests("TestSnapshotScripts")
	}

	if utils.HasFeature(t.Image, "WINDOWS") {
		passwordInst := &daisy.Instance{}
		passwordInst.Scopes = append(passwordInst.Scopes, "https://www.googleapis.com/auth/cloud-platform")
		windowsaccountVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "windowsaccount"}}, passwordInst)
		if err != nil {
			return err
		}
		windowsaccountVM.AddMetadata("enable-diagnostics", "true")
		windowsaccountVM.AddMetadata("enable-guest-attributes", "true")
		windowsaccountVM.RunTests("TestWindowsPasswordReset|TestDiagnostic")
	}

	if !utils.IsWindowsImage(t.Image.Name) {
		// Only test SSH host keys on non-Windows images.
		vm, err := t.CreateTestVM("sshkeytest")
		if err != nil {
			return fmt.Errorf("failed to create test VM: %w", err)
		}
		vm.AddScope("https://www.googleapis.com/auth/cloud-platform")
		vm.RunTests("TestSSHHostKeyExistence|TestSSHHostKeyTimingVsAgent|TestNetworkSetupCompletesBeforeAgentReady")
	}

	// This section is for testing MWLID. It is only run on non-Windows images and guest-agent derived images.
	if strings.Contains(t.Image.Name, "guest-agent") {
		project := t.Project.Name
		zone := t.Zone.Name
		projectNumber := "281997379984" // compute-image-test-pool-001 (where the CA pool was created)
		machineType := t.MachineType.Name

		poolID := "cic-pool1"
		namespaceID := "cic-ns"
		managedIdentityID := "cic-mi-sa-email1"

		workloadIdentity := strings.Join([]string{
			poolID, ".global.", projectNumber, ".workload.id.goog/ns/", namespaceID, "/sa/", managedIdentityID,
		}, "")

		mwlidInst := &daisy.Instance{}
		mwlidInst.Name = mwlidVMName // Use the constant name
		mwlidInst.MachineType = "projects/" + project + "/zones/" + zone + "/machineTypes/" + machineType
		mwlidInst.WorkloadIdentityConfig = &compute.WorkloadIdentityConfig{
			Identity:                   workloadIdentity,
			IdentityCertificateEnabled: true,
		}
		mwlidInst.ServiceAccounts = []*compute.ServiceAccount{
			{
				Email:  "default",
				Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"},
			},
		}
		mwlidInst.Tags = &compute.Tags{
			Items: []string{
				"vmexp-enable-shared-centralized-issuance-configs",
				"vmexp-enable-s2a",
			},
		}
		mwlidInst.Metadata = map[string]string{
			"vmDnsSetting": "ZonalOnly",
		}
		mwlidInst.NetworkInterfaces = []*compute.NetworkInterface{
			{
				Network: "projects/" + project + "/global/networks/default",
				AccessConfigs: []*compute.AccessConfig{
					{
						Type: "ONE_TO_ONE_NAT",
						Name: "External NAT",
					},
				},
			},
		}

		// When disks are defined in mwlidInst.Disks with InitializeParams,
		// the first argument to CreateTestVMMultipleDisks (additional disks) can be nil.
		mwlidVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: mwlidVMName, Type: diskType}}, mwlidInst)
		if err != nil {
			return fmt.Errorf("failed to create MWLID test VM: %v", err)
		}

		mwlidVM.RunTests("TestMWLIDCredentials")
	}
	return nil
}
