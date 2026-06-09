package imagebuilder

import (
	"net"
	"testing"
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
			t.Logf("Found active network interface: %s (Flags: %v)", iface.Name, iface.Flags)
		}
	}

	if !foundNonLoopback {
		t.Error("Failure: No non-loopback network interfaces were discovered on this system (equivalent command: 'ip -j a').")
	}

	if upCount == 0 {
		t.Error("Failure: Found network interfaces, but zero non-loopback interfaces are 'UP' (equivalent command: 'ip -j a').")
	}
}
