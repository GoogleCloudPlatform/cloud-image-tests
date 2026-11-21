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
	Name                = "guestagent"
	cloudPlatformScope  = "https://www.googleapis.com/auth/cloud-platform"
	mwlidVMName         = "mwlid" // Name of the VM used for mwlid tests.
	mwlidTestName       = "TestMWLIDCredentials"
	linuxHostKeyTests   = "TestSSHHostKeyExistence|TestSSHHostKeyTimingVsAgent|TestNetworkSetupCompletesBeforeAgentReady"
	windowsAccountTests = "TestWindowsPasswordReset|TestWindowsPasswordResetDifferentLocale|TestWindowsPasswordResetLegacyAgent|TestDiagnostic"
	windowsDCTestName   = "TestWindowsDomainControllerAccounts"
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	diskType := imagetest.DiskTypeNeeded(t.MachineType.Name)
	isWindows := utils.HasFeature(t.Image, "WINDOWS")

	if err := setupTelemetryDisabledVM(t, diskType); err != nil {
		return err
	}
	if err := setupTelemetryEnabledVM(t, diskType, isWindows); err != nil {
		return err
	}
	if err := setupWindowsDCVM(t, diskType); err != nil {
		return err
	}
	return setupMWLIDVM(t, diskType)
}

func createScopedVM(t *imagetest.TestWorkflow, name, diskType string) (*imagetest.TestVM, error) {
	inst := &daisy.Instance{}
	inst.Scopes = []string{cloudPlatformScope}
	inst.Name = name
	return t.CreateTestVMFromInstance(inst, []*compute.Disk{{Name: name, Type: diskType}})
}

// anchorTestRegex wraps a "|"-separated list of exact test names so Go's
// -test.run regexp does not also match tests with shared prefixes (such as
// TestWindowsPasswordReset matching TestWindowsPasswordResetLegacyAgent).
func anchorTestRegex(tests string) string {
	return fmt.Sprintf("^(%s)$", tests)
}

func telemetryDisabledTests(t *imagetest.TestWorkflow, diskType string) string {
	tests := "TestTelemetryDisabled"
	// Skip the snapshot tests on Windows client images, and any images using
	// hyperdisk. Run snapshot scripts here rather than on telemetryEnabled so
	// its Linux agent restart does not disturb boot-time readiness checks;
	// disable-guest-telemetry=true persists across the restart.
	if !utils.IsWindowsClient(t.Image.Name) && !strings.Contains(diskType, "hyperdisk") {
		tests += "|TestSnapshotScripts"
	}
	return tests
}

func telemetryEnabledTests(isWindows bool) string {
	tests := "TestTelemetryEnabled|TestServiceConfig"
	if isWindows {
		return tests + "|" + windowsAccountTests
	}
	return tests + "|" + linuxHostKeyTests
}

func setupTelemetryDisabledVM(t *imagetest.TestWorkflow, diskType string) error {
	tests := telemetryDisabledTests(t, diskType)
	if !t.ShouldRunTests(tests) {
		return nil
	}

	vm, err := createScopedVM(t, "telemetryDisabled", diskType)
	if err != nil {
		return err
	}
	vm.AddMetadata("disable-guest-telemetry", "true")
	vm.RunTests(anchorTestRegex(tests))
	return nil
}

func setupTelemetryEnabledVM(t *imagetest.TestWorkflow, diskType string, isWindows bool) error {
	tests := telemetryEnabledTests(isWindows)
	if !t.ShouldRunTests(tests) {
		return nil
	}

	vm, err := createScopedVM(t, "telemetryEnabled", diskType)
	if err != nil {
		return err
	}
	vm.AddMetadata("disable-guest-telemetry", "false")
	if isWindows {
		vm.AddMetadata("enable-diagnostics", "true")
		vm.AddMetadata("enable-guest-attributes", "true")
	}
	vm.RunTests(anchorTestRegex(tests))
	return nil
}

func supportsWindowsDomainControllerRole(img *compute.Image) bool {
	return utils.HasFeature(img, "WINDOWS") &&
		!utils.IsWindowsClient(img.Name) &&
		!utils.IsWindowsSQLImage(img.Name) &&
		!strings.Contains(img.Name, "-for-containers")
}

// isGuestAgentCandidateOrUnstablePkgImage reports whether img comes from a
// pipeline that installs pre-release guest-agent packages. Domain Controller
// account refusal only exists in candidate and unstable package builds until
// the updated google-compute-engine-windows package rolls out to public Windows
// Server images (see TODO(b/507998440) in setupWindowsDCVM):
//   - "guest-agent" in img.Name matches throwaway derivative images baked by
//     the guest-package-build/build-guest-agent Concourse job.
//   - "gce-unstable-pkg-test-images" in img.SelfLink matches pre-release test
//     images published to that project with unstable package repos enabled.
func isGuestAgentCandidateOrUnstablePkgImage(img *compute.Image) bool {
	return strings.Contains(img.Name, "guest-agent") ||
		strings.Contains(img.SelfLink, "gce-unstable-pkg-test-images")
}

func shouldRunWindowsDomainControllerTests(t *imagetest.TestWorkflow) bool {
	return supportsWindowsDomainControllerRole(t.Image) &&
		isGuestAgentCandidateOrUnstablePkgImage(t.Image) &&
		t.ShouldRunTests(windowsDCTestName)
}

func setupWindowsDCVM(t *imagetest.TestWorkflow, diskType string) error {
	// Active Directory Domain Controller account tests require the
	// AD-Domain-Services server role (not available on client, container, or
	// SQL Server images).
	// TODO(b/507998440): drop the guest-agent / gce-unstable-pkg-test-images
	// image filter once the updated google-compute-engine-windows package
	// reaches public Windows Server images.
	if !shouldRunWindowsDomainControllerTests(t) {
		return nil
	}

	dcInst := &daisy.Instance{
		InstanceBase: daisy.InstanceBase{
			Scopes: []string{cloudPlatformScope},
		},
		Metadata: map[string]string{
			imagetest.ShouldRebootDuringTest: "true",
		},
	}
	windowsDCVM, err := t.CreateTestVMFromInstance(dcInst, []*compute.Disk{{Name: "windowsdc", Type: diskType}})
	if err != nil {
		return err
	}
	if t.MachineType.Name == imagetest.DefaultMachineType {
		windowsDCVM.ForceMachineType("n1-standard-2")
	}
	windowsDCVM.AddMetadata("enable-guest-attributes", "true")
	windowsDCVM.AddMetadata("enable-windows-ssh", "true")
	windowsDCVM.AddMetadata("block-project-ssh-keys", "true")
	if err := windowsDCVM.Reboot(); err != nil {
		return err
	}
	windowsDCVM.RunTests(anchorTestRegex(windowsDCTestName))
	return nil
}

func setupMWLIDVM(t *imagetest.TestWorkflow, diskType string) error {
	// MWLID tests only run on guest-agent derived images and pre-release images
	// with the updated guest-agent.
	if !isGuestAgentCandidateOrUnstablePkgImage(t.Image) || !t.ShouldRunTests(mwlidTestName) {
		return nil
	}

	projectNumber := "281997379984" // compute-image-test-pool-001 (where the CA pool was created)
	poolID := "cic-pool1"
	namespaceID := "cic-ns"
	managedIdentityID := "cic-mi-sa-email1"

	workloadIdentity := strings.Join([]string{
		poolID, ".global.", projectNumber, ".workload.id.goog/ns/", namespaceID, "/sa/", managedIdentityID,
	}, "")

	mwlidInst := &daisy.Instance{}
	mwlidInst.Name = mwlidVMName // Use the constant name.
	mwlidInst.MachineType = t.MachineType.Name
	mwlidInst.WorkloadIdentityConfig = &compute.WorkloadIdentityConfig{
		Identity:                   workloadIdentity,
		IdentityCertificateEnabled: true,
	}
	mwlidInst.ServiceAccounts = []*compute.ServiceAccount{
		{
			Email:  "default",
			Scopes: []string{cloudPlatformScope},
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
			// Relative partial URL; daisy extends it with the workflow's project.
			Network: "global/networks/default",
			AccessConfigs: []*compute.AccessConfig{
				{
					Type: "ONE_TO_ONE_NAT",
					Name: "External NAT",
				},
			},
		},
	}

	mwlidVM, err := t.CreateTestVMFromInstance(mwlidInst, []*compute.Disk{{Name: mwlidVMName, Type: diskType}})
	if err != nil {
		return fmt.Errorf("creating MWLID test VM: %v", err)
	}
	mwlidVM.RunTests(anchorTestRegex(mwlidTestName))
	return nil
}
