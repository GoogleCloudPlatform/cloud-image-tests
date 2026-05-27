// Package imagebuilder is a CIT suite for testing customer images built by the GCE Image Builder.
package imagebuilder

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "imagebuilder"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	vm1, err := t.CreateTestVM("networkingandguestagent")
	if err != nil {
		return err
	}
	vm1.RunTests("TestNetworkInterfacesUp|TestNetworkInterfaceNaming|TestGoogleGuestAgentHealthy")

	if !utils.HasFeature(t.Image, "UEFI_COMPATIBLE") {
		return nil
	}
	vm4, err := t.CreateTestVM("secureboot")
	if err != nil {
		return err
	}
	vm4.EnableSecureBoot()
	vm4.RunTests("TestGuestSecureBoot")

	return nil
}
