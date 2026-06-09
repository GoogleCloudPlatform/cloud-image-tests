// Package imagebuilder is a CIT suite for testing customer images built by the Image Builder.
package imagebuilder

import (
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "imagebuilder"

// supportedSecureBootBaremetalMachineTypes is a list of baremetal machine types that support
// secure boot.
// https://docs.cloud.google.com/compute/shielded-vm/docs/shielded-vm#limitations
var (
	supportedSecureBootBaremetalMachineTypes = []string{"a4x", "c4a"}
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	vm1, err := t.CreateTestVM("networkingandguestagent")
	if err != nil {
		return err
	}
	vm1.RunTests("TestNetworkInterfacesUp|TestNetworkInterfaceNaming|TestGoogleGuestAgentHealthy")

	if utils.HasFeature(t.Image, "UEFI_COMPATIBLE") {
		// Only some baremetal machine types support secure boot.
		// https://docs.cloud.google.com/compute/shielded-vm/docs/shielded-vm#limitations
		runSecureBoot := !isBaremetal(t.MachineType.Name)
		if !runSecureBoot {
			for _, supportedType := range supportedSecureBootBaremetalMachineTypes {
				if strings.Contains(t.MachineType.Name, supportedType) {
					runSecureBoot = true
					break
				}
			}
		}

		if runSecureBoot {
			vm2, err := t.CreateTestVM("secureboot")
			if err != nil {
				return err
			}
			vm2.EnableSecureBoot()
			vm2.RunTests("TestGuestSecureBoot")
		}
	}

	return nil
}

func isBaremetal(machineType string) bool {
	return strings.Contains(machineType, "-metal")
}
