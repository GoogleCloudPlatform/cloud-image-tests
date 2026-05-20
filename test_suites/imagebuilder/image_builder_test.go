// Package imagebuilder is a CIT suite for testing customer images built by the GCE Image Builder.
package imagebuilder

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	secureBootFile  = "/sys/firmware/efi/efivars/SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c"
	setupModeFile   = "/sys/firmware/efi/efivars/SetupMode-8be4df61-93ca-11d2-aa0d-00e098032b8c"
	metadataCurlCmd = `curl -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/image`
)

func TestNetworkInterfacesUp(t *testing.T) {
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("Failed to retrieve network interfaces via net.Interfaces() (equivalent command: 'ip -j a'): %v", err)
	}

	// Validate that we have at least one valid, UP network interface
	var upCount int
	var foundNonLoopback bool

	for _, iface := range interfaces {
		// Skip the loopback interface via FlagLoopback or name "lo"
		if iface.Name == "lo" || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}

		foundNonLoopback = true

		// Check if the interface has the FlagUp set
		if (iface.Flags & net.FlagUp) != 0 {
			upCount++
			t.Logf("Found active network interface: %q (Flags: %q)", iface.Name, iface.Flags)
		}
	}

	if !foundNonLoopback {
		t.Error("Failure: No non-loopback network interfaces were discovered on this system (equivalent command: 'ip -j a').")
	}

	if upCount == 0 {
		t.Error("Failure: Found network interfaces, but zero non-loopback interfaces are 'UP' (equivalent command: 'ip -j a').")
	}
}

func TestNetworkInterfaceNaming(t *testing.T) {
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("Failed to retrieve network interfaces via net.Interfaces() (equivalent command: 'ip -j a'): %v", err)
	}

	// Ensure either traditional name (eth*) or predictable name (en*) is present
	var foundValidName bool
	for _, iface := range interfaces {
		// Skip loopback interfaces.
		if iface.Name == "lo" || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}

		if strings.HasPrefix(iface.Name, "eth") || strings.HasPrefix(iface.Name, "en") {
			foundValidName = true
			t.Logf("Found network interface with valid naming convention: %q", iface.Name)
		}
	}

	if !foundValidName {
		t.Error("Failure: No non-loopback interface was found following either traditional naming (eth*) or predictable naming (en*) conventions (equivalent command: 'ip -j a').")
	}
}

// TestGoogleGuestAgentHealthy checks if the Google Guest Agent service is active and fully operational (SubState=running).
func TestGoogleGuestAgentHealthy(t *testing.T) {
	ctx := utils.Context(t)

	// 1. Check if the service is active.
	cmdActive := exec.CommandContext(ctx, "systemctl", "is-active", "google-guest-agent.service")
	stdoutActive, err := cmdActive.Output()
	if err != nil {
		t.Errorf("Running `systemctl is-active google-guest-agent.service` expected `active`, but got: %q, Error: %v",
			strings.TrimSpace(string(stdoutActive)), err)
	}

	// 2. Query systemd for the exact substate of the service manager.
	cmdShow := exec.CommandContext(ctx, "systemctl", "show", "google-guest-agent.service", "--property=SubState")
	stdoutShow, err := cmdShow.Output()
	if err != nil {
		t.Fatalf("Failed while running `systemctl show google-guest-agent.service --property=SubState` to query systemd for SubState: %v. Stderr: %q", err, utils.ParseStderr(err))
	}

	// Output format is usually "SubState=running" or "SubState=dead"
	substate := strings.TrimSpace(string(stdoutShow))
	if !strings.Contains(substate, "SubState=running") {
		t.Errorf("Health Check Failed: Agent is not fully operational after running 'systemctl show google-guest-agent.service --property=SubState'. Found %q (expected SubState=running)", substate)
	} else {
		t.Log("Health Check Passed: google-guest-agent is actively running.")
	}
}

func TestGuestSecureBoot(t *testing.T) {
	if err := testLinuxGuestSecureBoot(t); err != nil {
		t.Fatalf("[FAILED] error running SecureBoot test: %v", err)
	}
	t.Logf("Secure Boot is enabled and configured correctly.")
}

func mountEFIVarsCOS(t *testing.T) error {
	t.Helper()

	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("[FAILED] unable to get image from metadata with equivalent command %q: %v", metadataCurlCmd, err)
	}

	if !utils.IsCOS(image) {
		return nil
	}

	if _, err := os.Stat(secureBootFile); !os.IsNotExist(err) {
		return nil
	}

	cmd := exec.CommandContext(ctx, "mount", "-t", "efivarfs", "efivarfs", "/sys/firmware/efi/efivars/")
	_, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to mount EFI vars with command %q: %v", cmd.String(), err)
	}
	return nil
}

func testLinuxGuestSecureBoot(t *testing.T) error {
	if err := mountEFIVarsCOS(t); err != nil {
		return err
	}

	if _, err := os.Stat(secureBootFile); os.IsNotExist(err) {
		return fmt.Errorf("failed to stat file %q: secureboot efi var is missing", secureBootFile)
	}
	data, err := os.ReadFile(secureBootFile)
	if err != nil {
		return fmt.Errorf("failed reading secure boot file %q: %v", secureBootFile, err)
	}
	// https://www.kernel.org/doc/Documentation/ABI/stable/sysfs-firmware-efi-vars
	secureBootMode := data[len(data)-1]
	// https://uefi.org/specs/UEFI/2.9_A/32_Secure_Boot_and_Driver_Signing.html#firmware-os-key-exchange-creating-trust-relationships
	// If setup mode is not 0 secure boot isn't actually enabled because no PK is enrolled.
	if _, err = os.Stat(setupModeFile); os.IsNotExist(err) {
		return fmt.Errorf("failed to stat file %q: setupmode efi var is missing", setupModeFile)
	}
	data, err = os.ReadFile(setupModeFile)
	if err != nil {
		return fmt.Errorf("failed reading setup mode file %q: %v", setupModeFile, err)
	}
	setupMode := data[len(data)-1]
	if secureBootMode != 1 || setupMode != 0 {
		return fmt.Errorf("secure boot is not enabled, found secureboot mode: %c (want 1) and setup mode: %c (want 0)", secureBootMode, setupMode)
	}
	return nil
}

func TestSuspend(t *testing.T) {
	marker := "/var/suspend-test-start"
	if _, err := os.Stat(marker); err != nil && !os.IsNotExist(err) {
		t.Fatalf("`stat %q` failed: could not determine if suspend testing has already started: %v", marker, err)
	} else if err == nil {
		t.Fatal("unexpected reboot during suspend test")
	}
	err := os.WriteFile(marker, nil, 0777)
	if err != nil {
		t.Fatalf("could not mark beginning of suspend testing by creating %q: %v", marker, err)
	}
	ctx := utils.Context(t)
	prj, zone, err := utils.GetProjectZone(ctx)
	if err != nil {
		t.Fatalf("could not find project and zone by querying metadata: %v", err)
	}
	inst, err := utils.GetInstanceName(ctx)
	if err != nil {
		t.Fatalf("could not get instance name by querying metadata: %v", err)
	}

	client, err := utils.GetDaisyClient(ctx)
	if err != nil {
		t.Fatalf("could not make compute api client: %v", err)
	}

	err = client.Suspend(prj, zone, inst)
	if err != nil {
		// We can't really check the operation error here, we want to attempt to wait until its suspended but the wait operation will likely error out due to being interrupted by the suspension
		if !strings.Contains(err.Error(), "operation failed") && !strings.Contains(err.Error(), "failed to get zone operation") {
			t.Fatalf("could not suspend self by calling Compute API: %v", err)
		}
	}

	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("`stat %q` failed: could not confirm suspend testing has started ok: %v", marker, err)
	}
	_, err = http.Get("https://cloud.google.com")
	if err != nil {
		t.Errorf("no network connectivity after resume: %v", err)
	}
	t.Log("Network connectivity is good after resuming")
}
