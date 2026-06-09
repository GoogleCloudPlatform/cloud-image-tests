// Package imagebuilder is a CIT suite for testing customer images built by the Image Builder.
package imagebuilder

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
)

// Name is the name of the test package. It must match the directory name.
var Name = "imagebuilder"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	vm1, err := t.CreateTestVM("networkinterfaces")
	if err != nil {
		return err
	}
	vm1.RunTests("TestNetworkInterfacesUp|TestNetworkInterfaceNaming")
	return nil
}
