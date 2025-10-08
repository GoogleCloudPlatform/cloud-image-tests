// Copyright 2025 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package wsfc is a CIT suite for testing Windows Server Failover Cluster
// functionality.
package wsfc

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "wsfc"

const wsfcAgentPort = "59998"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if !utils.HasFeature(t.Image, "WINDOWS") {
		t.Skip("Skipping wsfc test for non-Windows images.")
	}

	vm, err := t.CreateTestVM("wsfc")
	if err != nil {
		return err
	}
	vm.AddMetadata("enable-wsfc", "true")
	vm.AddMetadata("wcf-agent-port", wsfcAgentPort)
	vm.RunTests("TestHealthCheckAgent")

	return nil
}
