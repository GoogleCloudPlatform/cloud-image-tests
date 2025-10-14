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

// Package acceleratornccl runs nccl-tests on accelerator images.
package acceleratornccl

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/acceleratorutils"
)

var (
	// Name is the name of the test package. It must match the directory name.
	Name = "acceleratornccl"
)

// TestSetup sets up test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	t.LockProject()
	nics, err := acceleratorutils.CreateNetwork(t)
	if err != nil {
		return err
	}
	vm, err := acceleratorutils.CreateVM(t, "ncclVM", nics)
	if err != nil {
		return err
	}
	vm.RunTests("TestNCCL")
	return nil
}
