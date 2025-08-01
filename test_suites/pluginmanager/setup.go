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

// Package pluginmanager contains the test suite for testing the Plugin Manager
// related features.
package pluginmanager

import (
	"github.com/GoogleCloudPlatform/cloud-image-tests"
)

// Name is the name of the test package. It must match the directory name.
var Name = "pluginmanager"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	defaultVM, err := t.CreateTestVM("plugincleanup")
	if err != nil {
		return err
	}
	defaultVM.RunTests("TestPluginCleanup")
	pluginStopVM, err := t.CreateTestVM("pluginstop")
	if err != nil {
		return err
	}
	pluginStopVM.RunTests("TestCorePluginStop")
	return nil
}
