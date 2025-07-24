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

// Package mdsroutes is a CIT suite for testing network routes to the
// metadata server.
package mdsroutes

import (
	"fmt"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/exceptions"
)

// Name is the name of the test package. It must match the directory name.
var Name = "mdsroutes"

var (
	unsupportedImages = []exceptions.Exception{
		{
			Match: exceptions.ImageCOS,
		},
		{
			// TODO(b/428199320): Remove this once the bug is fixed.
			Match:   exceptions.ImageSLES,
			Version: 15,
			Type:    exceptions.Equal,
		},
	}
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if exceptions.HasMatch(t.Image.Family, unsupportedImages) {
		t.Skip(fmt.Sprintf("MDS routes not supported on %s", t.Image.Family))
		return nil
	}

	network1, err := t.CreateNetwork("network-1", false)
	if err != nil {
		return err
	}
	subnetwork1, err := network1.CreateSubnetwork("subnetwork-1", "10.128.0.0/20")
	if err != nil {
		return err
	}
	subnetwork1.AddSecondaryRange("secondary-range", "10.14.0.0/16")

	network2, err := t.CreateNetwork("network-2", false)
	if err != nil {
		return err
	}
	subnetwork2, err := network2.CreateSubnetwork("subnetwork-2", "192.168.0.0/16")
	if err != nil {
		return err
	}

	// VM for multiNIC
	multinicVM, err := t.CreateTestVM("multinic")
	if err != nil {
		return err
	}
	if err := multinicVM.AddCustomNetwork(network1, subnetwork1); err != nil {
		return err
	}
	if err := multinicVM.AddCustomNetwork(network2, subnetwork2); err != nil {
		return err
	}
	if err := multinicVM.SetPrivateIP(network2, "192.168.0.2"); err != nil {
		return err
	}
	if err := multinicVM.AddAliasIPRanges("10.14.8.0/24", "secondary-range"); err != nil {
		return err
	}

	tests := []string{"TestDNS", "TestMetadataPath"}

	// TODO(b/428199320): Remove this once the bug is fixed.
	if !exceptions.MatchAll(t.Image.Family, exceptions.ImageUbuntu, exceptions.Exception{Version: 2204}) {
		tests = append(tests, "TestMDSRoutes")
		return nil
	}
	multinicVM.RunTests(strings.Join(tests, "|"))

	return nil
}
