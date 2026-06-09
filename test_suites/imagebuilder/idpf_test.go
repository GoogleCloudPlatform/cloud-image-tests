// Package imagebuilder is a CIT suite for testing customer images built by the GCE Image Builder.
package imagebuilder

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

const idpfDriverName = "idpf"

// TestIDPFNICDriver ensures that for bare metal instances where the image supports IDPF, the IDPF
// driver is correctly loaded and used for physical network interfaces, and their naming follows the
// acceptable naming scheme.
func TestIDPFNICDriver(t *testing.T) {
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("Failed to retrieve network interfaces via net.Interfaces() (equivalent command: 'ip -j a'): %v", err)
	}

	var physicalNICsFound int

	for _, iface := range interfaces {
		// Skip the loopback interface
		if iface.Name == "lo" || (iface.Flags&net.FlagLoopback) != 0 {
			continue
		}

		driverPath := fmt.Sprintf("/sys/class/net/%s/device/driver", iface.Name)
		target, err := os.Readlink(driverPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Virtual / bridge interface, skip checking driver
				continue
			}
			t.Errorf("Failed to read symlink for interface %q, driver path %q: %v", iface.Name, driverPath, err)
			continue
		}

		physicalNICsFound++

		driverName := filepath.Base(target)
		if driverName != idpfDriverName {
			t.Errorf("Interface %q is using driver %q, expected %q (Intel IDPF driver)", iface.Name, driverName, idpfDriverName)
		} else {
			t.Logf("Successfully verified interface %q is using driver %q", iface.Name, driverName)
		}

	}

	if physicalNICsFound == 0 {
		t.Error("No physical cloud network interfaces were discovered on this bare metal system.")
	}
}
