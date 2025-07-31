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

// Package nicsetup contains the setup logic for the nicsetup test suite.
// This primarily tests different network configurations for IPv4, dual stack,
// and IPv6only networks.
package nicsetup

import (
	"flag"
	"fmt"
	"slices"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

var (
	// Name is the name of the test package. It must match the directory name.
	Name = "nicsetup"

	// vmtype is the VM type to use for the test. Options are 'multi', 'single', and 'both'.
	// Default is 'both'. 'single' will create only single NIC VMs. 'multi' will
	// create only multi NIC VMs. 'both' will create both single and multi NIC VMs.
	vmtype = flag.String("nicsetup_vmtype", "both", "The VM type to use for the test. Options are 'multi', 'single', or 'both'. Default is 'both'.")

	// possibleVMTypes is the list of possible VM types for the test.
	possibleVMTypes = []string{"multi", "single", "both"}
)

const (
	pingVMIPv4 = "10.0.1.128"
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if utils.HasFeature(t.Image, "WINDOWS") {
		return nil
	}

	if !slices.Contains(possibleVMTypes, *vmtype) {
		return fmt.Errorf("invalid vmtype: %s\nMust be one of: %v", *vmtype, possibleVMTypes)
	}

	// Create an IPv4 network.
	network1, err := t.CreateNetwork("network1", false)
	if err != nil {
		return err
	}
	ipv4Subnetwork1, err := network1.CreateSubnetwork("ipv4", "10.128.0.0/24")
	if err != nil {
		return err
	}
	dualstackSubnetwork1, err := network1.CreateSubnetworkFromDaisySubnetwork(&daisy.Subnetwork{
		Subnetwork: compute.Subnetwork{
			Name:           "dual",
			IpCidrRange:    "10.128.1.0/24",
			StackType:      "IPV4_IPV6",
			Ipv6AccessType: "EXTERNAL",
		},
	})
	if err != nil {
		return err
	}
	ipv6Subnetwork1, err := network1.CreateSubnetworkFromDaisySubnetwork(&daisy.Subnetwork{
		Subnetwork: compute.Subnetwork{
			Name:           "ipv6",
			StackType:      "IPV6_ONLY",
			Ipv6AccessType: "EXTERNAL",
		},
	})
	if err != nil {
		return err
	}

	// List of all VMs to create.
	// List of VMs with IPv4 primary NIC.
	var allVMs []*imagetest.TestVM
	// List of VMs with IPv4 secondary NIC.
	var allMultiVMs []*imagetest.TestVM

	// The following are all single NIC VMs.
	if *vmtype != "multi" {
		ipv4VM1, err := t.CreateTestVM("ipv4")
		if err != nil {
			return err
		}
		dualstackVM1, err := t.CreateTestVM("dual")
		if err != nil {
			return err
		}
		ipv6VM1, err := t.CreateTestVM("ipv6")
		if err != nil {
			return err
		}

		// Add networks to VMs.
		ipv4VM1.AddCustomNetworkWithStackType(network1, ipv4Subnetwork1, "IPV4_ONLY", "")
		dualstackVM1.AddCustomNetworkWithStackType(network1, dualstackSubnetwork1, "IPV4_IPV6", "")
		ipv6VM1.AddCustomNetworkWithStackType(network1, ipv6Subnetwork1, "IPV6_ONLY", "EXTERNAL")

		allSingleVMs := []*imagetest.TestVM{ipv4VM1, dualstackVM1, ipv6VM1}
		for _, vm := range allSingleVMs {
			vm.AddMetadata("network-interfaces-count", "1")
		}
		allVMs = append(allVMs, allSingleVMs...)
	}

	// The following are all multi NIC VMs.
	if *vmtype != "single" {
		// Create a second network. The subnetworks are internal IPv6 because
		// secondary NICs don't have external connectivity by default. We specify
		// AutoCreateSubnetworks explicitly to avoid creating legacy networks.
		falseValue := false
		network2, err := t.CreateNetworkFromDaisyNetwork(&daisy.Network{
			Network: compute.Network{
				Name:                  "network2",
				EnableUlaInternalIpv6: true,
			},
			AutoCreateSubnetworks: &falseValue,
		})
		if err != nil {
			return err
		}
		ipv4Subnetwork2, err := network2.CreateSubnetwork("ipv4-2", "10.0.0.0/24")
		if err != nil {
			return err
		}
		dualstackSubnetwork2, err := network2.CreateSubnetworkFromDaisySubnetwork(&daisy.Subnetwork{
			Subnetwork: compute.Subnetwork{
				Name:           "dual-2",
				IpCidrRange:    "10.0.1.0/24",
				StackType:      "IPV4_IPV6",
				Ipv6AccessType: "INTERNAL",
			},
		})
		if err != nil {
			return err
		}
		ipv6Subnetwork2, err := network2.CreateSubnetworkFromDaisySubnetwork(&daisy.Subnetwork{
			Subnetwork: compute.Subnetwork{
				Name:           "ipv6-2",
				StackType:      "IPV6_ONLY",
				Ipv6AccessType: "INTERNAL",
			},
		})
		if err != nil {
			return err
		}

		// Create the VMs.
		ipv4ipv4, err := t.CreateTestVM("ipv4ipv4")
		if err != nil {
			return err
		}
		ipv4dual, err := t.CreateTestVM("ipv4dual")
		if err != nil {
			return err
		}
		ipv4ipv6, err := t.CreateTestVM("ipv4ipv6")
		if err != nil {
			return err
		}
		dualipv4, err := t.CreateTestVM("dualipv4")
		if err != nil {
			return err
		}
		dualdual, err := t.CreateTestVM("dualdual")
		if err != nil {
			return err
		}
		dualipv6, err := t.CreateTestVM("dualipv6")
		if err != nil {
			return err
		}
		ipv6ipv4, err := t.CreateTestVM("ipv6ipv4")
		if err != nil {
			return err
		}
		ipv6dual, err := t.CreateTestVM("ipv6dual")
		if err != nil {
			return err
		}
		ipv6ipv6, err := t.CreateTestVM("ipv6ipv6")
		if err != nil {
			return err
		}

		// Add networks to VMs.
		ipv4ipv4.AddCustomNetworkWithStackType(network1, ipv4Subnetwork1, "IPV4_ONLY", "")
		ipv4ipv4.AddCustomNetworkWithStackType(network2, ipv4Subnetwork2, "IPV4_ONLY", "")

		ipv4dual.AddCustomNetworkWithStackType(network1, ipv4Subnetwork1, "IPV4_ONLY", "")
		ipv4dual.AddCustomNetworkWithStackType(network2, dualstackSubnetwork2, "IPV4_IPV6", "INTERNAL")

		ipv4ipv6.AddCustomNetworkWithStackType(network1, ipv4Subnetwork1, "IPV4_ONLY", "")
		ipv4ipv6.AddCustomNetworkWithStackType(network2, ipv6Subnetwork2, "IPV6_ONLY", "INTERNAL")

		dualipv4.AddCustomNetworkWithStackType(network1, dualstackSubnetwork1, "IPV4_IPV6", "")
		dualipv4.AddCustomNetworkWithStackType(network2, ipv4Subnetwork2, "IPV4_ONLY", "INTERNAL")

		dualdual.AddCustomNetworkWithStackType(network1, dualstackSubnetwork1, "IPV4_IPV6", "")
		dualdual.AddCustomNetworkWithStackType(network2, dualstackSubnetwork2, "IPV4_IPV6", "INTERNAL")

		dualipv6.AddCustomNetworkWithStackType(network1, dualstackSubnetwork1, "IPV4_IPV6", "")
		dualipv6.AddCustomNetworkWithStackType(network2, ipv6Subnetwork2, "IPV6_ONLY", "INTERNAL")

		ipv6ipv4.AddCustomNetworkWithStackType(network1, ipv6Subnetwork1, "IPV6_ONLY", "EXTERNAL")
		ipv6ipv4.AddCustomNetworkWithStackType(network2, ipv4Subnetwork2, "IPV4_ONLY", "")

		ipv6dual.AddCustomNetworkWithStackType(network1, ipv6Subnetwork1, "IPV6_ONLY", "EXTERNAL")
		ipv6dual.AddCustomNetworkWithStackType(network2, dualstackSubnetwork2, "IPV4_IPV6", "INTERNAL")

		ipv6ipv6.AddCustomNetworkWithStackType(network1, ipv6Subnetwork1, "IPV6_ONLY", "EXTERNAL")
		ipv6ipv6.AddCustomNetworkWithStackType(network2, ipv6Subnetwork2, "IPV6_ONLY", "INTERNAL")

		allMultiVMs = append(allMultiVMs, ipv4ipv4, ipv4dual, ipv4ipv6, dualipv4, dualdual, dualipv6, ipv6ipv4, ipv6dual, ipv6ipv6)
		allVMs = append(allVMs, allMultiVMs...)
		for _, vm := range allMultiVMs {
			vm.AddMetadata("network-interfaces-count", "2")
		}
	}

	for _, vm := range allVMs {
		var tests []string
		if slices.Contains(allMultiVMs, vm) {
			tests = append(tests, "TestSecondaryNIC")
		}
		tests = append(tests, "TestPrimaryNIC")
		vm.RunTests(strings.Join(tests, "|"))
	}
	return nil
}
