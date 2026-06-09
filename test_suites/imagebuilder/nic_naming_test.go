// Package imagebuilder is a CIT suite for testing customer images built by the GCE Image Builder.
package imagebuilder

import (
	"net"
	"strings"
	"testing"
)

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
