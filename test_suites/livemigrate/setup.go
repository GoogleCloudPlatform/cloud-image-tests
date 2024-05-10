// Copyright 2024 Google LLC.
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

// Package livemigrate is a CIT suite for testing standard live migration, not
// confidential vm live migration. See the cvm suite for the latter.
package livemigrate

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "livemigrate"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	lm := &daisy.Instance{}
	lm.Scopes = append(lm.Scopes, "https://www.googleapis.com/auth/cloud-platform")
	lm.Scheduling = &compute.Scheduling{OnHostMaintenance: "MIGRATE"}
	lmvm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "livemigrate"}}, lm)
	if err != nil {
		return err
	}
	lmvm.RunTests("TestLiveMigrate")
	return nil
}
