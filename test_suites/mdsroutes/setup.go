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
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "mdsroutes"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if strings.Contains(t.Image.Family, "cos") {
		t.Skip("MDS routes not supported on COS")
		return nil
	}

	// TODO(b/428199320): Remove this once the bug is fixed.
	if strings.Contains(t.Image.Family, "sles") {
		t.Skip("MDS routes not supported on SLES")
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

	var tests []string
	dnsTest := "TestDNS"
	if utils.HasFeature(t.Image, "WINDOWS") {
		dnsTest = "TestWindowsDNS"
	}
	tests = append(tests, dnsTest, "TestMetadataPath")

	// TODO(b/428199320): Remove this once the bug is fixed.
	if !strings.Contains(t.Image.Family, "ubuntu-2204") {
		tests = append(tests, "TestMDSRoutes")
		return nil
	}
	multinicVM.RunTests(strings.Join(tests, "|"))

	return nil
}
