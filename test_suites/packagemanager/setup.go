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

// Package packagemanager is a CIT suite for testing package manager functionality.
package packagemanager

import (
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "packagemanager"

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	network1, err := t.CreateNetwork("network-1", false)
	if err != nil {
		return err
	}
	ipv4OnlySubnet := &daisy.Subnetwork{}
	ipv4OnlySubnet.Name = "ipv4Only"
	ipv4OnlySubnet.StackType = "IPV4_ONLY"
	ipv4OnlySubnet.IpCidrRange = "10.128.0.0/24"
	_, err = network1.CreateSubnetworkFromDaisySubnetwork(ipv4OnlySubnet)
	if err != nil {
		return err
	}

	dualStackSubnet := &daisy.Subnetwork{}
	dualStackSubnet.Name = "dualStack"
	dualStackSubnet.StackType = "IPV4_IPV6"
	dualStackSubnet.IpCidrRange = "10.128.1.0/24"
	dualStackSubnet.Ipv6AccessType = "EXTERNAL"
	_, err = network1.CreateSubnetworkFromDaisySubnetwork(dualStackSubnet)
	if err != nil {
		return err
	}

	dualStack := &daisy.Instance{}
	dualStack.NetworkInterfaces = []*compute.NetworkInterface{
		{
			StackType:  "IPV4_IPV6",
			Network:    "network-1",
			Subnetwork: "dualStack",
		},
	}
	dualStackVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "dualstack"}}, dualStack)
	if err != nil {
		return err
	}
	dualStackVM.RunTests("TestRepoReachabilityDualStack")

	ipv4Only := &daisy.Instance{}
	ipv4Only.NetworkInterfaces = []*compute.NetworkInterface{
		{
			StackType:  "IPV4_ONLY",
			Network:    "network-1",
			Subnetwork: "ipv4Only",
		},
	}
	ipv4OnlyVM, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: "ipv4only"}}, ipv4Only)
	if err != nil {
		return err
	}
	ipv4OnlyVM.RunTests("TestRepoReachabilityIPv4Only")

	// Running the test only on guest-agent images to avoid noise and ensure its
	// not flaky.
	if strings.Contains(t.Image.Name, "guest-agent") {
		removeAgentVM, err := t.CreateTestVM("removeagent")
		if err != nil {
			return err
		}
		removeAgentVM.RunTests("TestRemoveAgentSetup")
	}
	return nil
}
