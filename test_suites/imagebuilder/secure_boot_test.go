package imagebuilder

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	secureBootFile  = "/sys/firmware/efi/efivars/SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c"
	setupModeFile   = "/sys/firmware/efi/efivars/SetupMode-8be4df61-93ca-11d2-aa0d-00e098032b8c"
	metadataCurlCmd = `curl -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/image`
)

func TestGuestSecureBoot(t *testing.T) {
	if err := testLinuxGuestSecureBoot(t); err != nil {
		t.Fatalf("[FAILED] error running SecureBoot test: %v", err)
	}
	t.Logf("Secure Boot is enabled and configured correctly.")
}

func mountEFIVarsCOS(t *testing.T) error {
	t.Helper()
	ctx := utils.Context(t)
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Logf("Could not read os-release: %v", err)
		return nil
	}
	if !utils.IsCOS(string(content)) {
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
