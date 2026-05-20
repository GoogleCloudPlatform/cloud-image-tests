// Package imagebuilder is a CIT suite for testing customer images built by the GCE Image Builder.
package imagebuilder

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
)

// Name is the name of the test package. It must match the directory name.
var Name = "imagebuilder"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	vm1, err := t.CreateTestVM("networkinterfacesup")
	if err != nil {
		return err
	}
	vm1.RunTests("TestNetworkInterfacesUp")

	vm2, err := t.CreateTestVM("networkinterfacenaming")
	if err != nil {
		return err
	}
	vm2.RunTests("TestNetworkInterfaceNaming")

	return nil
}
