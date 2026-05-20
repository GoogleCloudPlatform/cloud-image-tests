// Package imagebuilder is a CIT suite for testing customer images built by the GCE Image Builder.
package imagebuilder

import (
	"encoding/json"
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

// IPAddressOutput mirrors the structure returned by `ip -j a`
type IPAddressOutput struct {
	Ifindex   int      `json:"ifindex"`
	Ifname    string   `json:"ifname"`
	Flags     []string `json:"flags"`
	Operstate string   `json:"operstate"` // Can be "UP", "DOWN", "UNKNOWN", etc.
	Address   string   `json:"address"`   // MAC address
}

func TestNetworkInterfacesUp(t *testing.T) {
	ctx := utils.Context(t)
	cmd := exec.CommandContext(ctx, "ip", "-j", "a")
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to execute 'ip -j a'. Error: %v. Stderr: %s. Stdout: %s", err, utils.ParseStderr(err), string(stdout))
	}

	// 2. Unmarshal the JSON into our Go struct slice
	var interfaces []IPAddressOutput
	if err := json.Unmarshal(stdout, &interfaces); err != nil {
		t.Fatalf("Failed to parse JSON output from 'ip -j a': %v. Raw output: %s", err, string(stdout))
	}

	// 3. Validate that we have at least one valid, UP network interface
	var upCount int
	var foundNonLoopback bool

	for _, iface := range interfaces {
		// Skip the loopback interface (lo) as it doesn't represent external connectivity
		if iface.Ifname == "lo" || contains(iface.Flags, "LOOPBACK") {
			continue
		}

		foundNonLoopback = true

		// Check if the operational state is explicitly UP
		if strings.ToUpper(iface.Operstate) == "UP" {
			upCount++
			t.Logf("Found active network interface: %s (State: %s)", iface.Ifname, iface.Operstate)
		}
	}

	if !foundNonLoopback {
		t.Error("Failure: No non-loopback network interfaces were discovered on this system by running 'ip -j a'.")
	}

	if upCount == 0 {
		t.Error("Failure: Found network interfaces, but zero non-loopback interfaces are 'UP' when running 'ip -j a'.")
	}
}

func TestNetworkInterfaceNaming(t *testing.T) {
	ctx := utils.Context(t)
	cmd := exec.CommandContext(ctx, "ip", "-j", "a")
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to execute 'ip -j a'. Error: %v. Stderr: %s. Stdout: %s", err, utils.ParseStderr(err), string(stdout))
	}

	// 2. Unmarshal the JSON into our Go struct slice
	var interfaces []IPAddressOutput
	if err := json.Unmarshal(stdout, &interfaces); err != nil {
		t.Fatalf("Failed to parse JSON output from 'ip -j a': %v. Raw output: %s", err, string(stdout))
	}

	// Ensure either traditional name (eth*) or predictable name (en*) is present
	var foundValidName bool
	for _, iface := range interfaces {
		// Skip loopback interfaces.
		if iface.Ifname == "lo" || contains(iface.Flags, "LOOPBACK") {
			continue
		}

		if strings.HasPrefix(iface.Ifname, "eth") || strings.HasPrefix(iface.Ifname, "en") {
			foundValidName = true
			t.Logf("Found network interface with valid naming convention: %s", iface.Ifname)
		}
	}

	if !foundValidName {
		t.Error("Failure: No non-loopback interface was found following either traditional naming (eth*) or predictable naming (en*) conventions by running 'ip -j a'.")
	}
}

// Helper function to check if a slice contains a specific string flag
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, item) {
			return true
		}
	}
	return false
}
