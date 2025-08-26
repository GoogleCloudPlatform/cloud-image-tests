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
	"strconv"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/exceptions"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

const (
	pingVMIPv4     = "10.0.0.128"
	supportIpv6Key = "supports-ipv6"
)

var (
	// Name is the name of the test package. It must match the directory name.
	Name = "nicsetup"

	// vmtype is the VM type to use for the test. Options are 'multi', 'single', and 'both'.
	// Default is 'both'. 'single' will create only single NIC VMs. 'multi' will
	// create only multi NIC VMs. 'both' will create both single and multi NIC VMs.
	vmtype = flag.String("nicsetup_vmtype", "both", "The VM type to use for the test. Options are 'multi', 'single', or 'both'. 'multi' will create only multi-NIC VMs. 'single' will only create single-NIC VMs. 'both' will create both.")

	// possibleVMTypes is the list of possible VM types for the test.
	possibleVMTypes = []string{"multi", "single", "both"}

	// ipv6Exceptions are the list of images that do not support IPv6.
	ipv6Exceptions = []exceptions.Exception{
		exceptions.Exception{
			Match:   exceptions.ImageSLES,
			Version: 12,
			Type:    exceptions.Equal,
		},
		exceptions.Exception{
			// TODO(b/440780139): rhel-9-0-sap-ha and rhel-8-6-sap-ha does not work
			// with IPv6. Remove the exception if it is expected to work and bug is
			// fixed.
			Match: "rhel-(?:9-0|8-6)-sap-ha",
		},
	}
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if utils.HasFeature(t.Image, "WINDOWS") {
		return nil
	}

	// Verify the VM type for the test.
	if !slices.Contains(possibleVMTypes, *vmtype) {
		return fmt.Errorf("invalid vmtype: %s\nMust be one of: %v", *vmtype, possibleVMTypes)
	}

	// Verify that the image supports IPv6.
	supportsIpv6 := !exceptions.HasMatch(t.Image.Name, ipv6Exceptions)

	// Create an primary network.
	network1, err := t.CreateNetwork("network1", false)
	if err != nil {
		return err
	}
	subnetwork1, err := network1.CreateSubnetworkFromDaisySubnetwork(&daisy.Subnetwork{
		Subnetwork: compute.Subnetwork{
			Name:           "dual",
			IpCidrRange:    "10.128.0.0/24",
			StackType:      "IPV4_IPV6",
			Ipv6AccessType: "EXTERNAL",
		},
	})
	if err != nil {
		return err
	}

	// List of all VMs to create.
	var allVMs []*imagetest.TestVM
	// List of all multi NIC VMs.
	var allMultiVMs []*imagetest.TestVM

	// The following are all single NIC VMs.
	if *vmtype != "multi" {
		var allSingleVMs []*imagetest.TestVM

		ipv4VM1, err := t.CreateTestVM("ipv4")
		if err != nil {
			return err
		}
		ipv4VM1.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV4_ONLY", "")
		allSingleVMs = append(allSingleVMs, ipv4VM1)

		// Only create dual stack and IPv6 only VMs if the image supports IPv6.
		if supportsIpv6 {
			dualstackVM1, err := t.CreateTestVM("dual")
			if err != nil {
				return err
			}
			ipv6VM1, err := t.CreateTestVM("ipv6")
			if err != nil {
				return err
			}
			dualstackVM1.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV4_IPV6", "")
			ipv6VM1.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV6_ONLY", "EXTERNAL")

			allSingleVMs = append(allSingleVMs, dualstackVM1, ipv6VM1)
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
		subnetwork2, err := network2.CreateSubnetworkFromDaisySubnetwork(&daisy.Subnetwork{
			Subnetwork: compute.Subnetwork{
				Name:           "dual-2",
				IpCidrRange:    "10.0.0.0/24",
				StackType:      "IPV4_IPV6",
				Ipv6AccessType: "INTERNAL",
			},
		})
		if err != nil {
			return err
		}

		// Add firewall rules allowing TCP traffic.
		if err := network2.CreateFirewallRule("allow-connection-ipv4", "tcp", nil, []string{"0.0.0.0/0"}); err != nil {
			return err
		}
		if err := network2.CreateFirewallRule("allow-connection-ipv6", "tcp", nil, []string{"0::0/0"}); err != nil {
			return err
		}

		// Create the ping VMs. There's one for each subnetwork.
		pingVM, err := t.CreateTestVM("ping")
		if err != nil {
			return err
		}
		if supportsIpv6 {
			pingVM.AddCustomNetworkWithStackType(network2, subnetwork2, "IPV4_IPV6", "INTERNAL")
		} else {
			pingVM.AddCustomNetworkWithStackType(network2, subnetwork2, "IPV4_ONLY", "")
		}
		pingVM.SetPrivateIP(network2, pingVMIPv4)
		pingVM.AddScope("https://www.googleapis.com/auth/compute") // Compute scope is needed for setting metadata.
		pingVM.AddMetadata(supportIpv6Key, strconv.FormatBool(supportsIpv6))
		pingVM.RunTests("TestEmpty")

		// Create the VMs.
		ipv4ipv4, err := t.CreateTestVM("ipv4ipv4")
		if err != nil {
			return err
		}
		ipv4ipv4.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV4_ONLY", "")
		ipv4ipv4.AddCustomNetworkWithStackType(network2, subnetwork2, "IPV4_ONLY", "")
		allMultiVMs = append(allMultiVMs, ipv4ipv4)

		if supportsIpv6 {
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

			// Add networks to VMs. Primary NIC must be EXTERNAL IPv6 for tests to work.
			ipv4dual.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV4_ONLY", "")
			ipv4dual.AddCustomNetworkWithStackType(network2, subnetwork2, "IPV4_IPV6", "INTERNAL")

			ipv4ipv6.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV4_ONLY", "")
			ipv4ipv6.AddCustomNetworkWithStackType(network2, subnetwork2, "IPV6_ONLY", "INTERNAL")

			dualipv4.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV4_IPV6", "")
			dualipv4.AddCustomNetworkWithStackType(network2, subnetwork2, "IPV4_ONLY", "INTERNAL")

			dualdual.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV4_IPV6", "")
			dualdual.AddCustomNetworkWithStackType(network2, subnetwork2, "IPV4_IPV6", "INTERNAL")

			dualipv6.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV4_IPV6", "")
			dualipv6.AddCustomNetworkWithStackType(network2, subnetwork2, "IPV6_ONLY", "INTERNAL")

			ipv6ipv4.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV6_ONLY", "EXTERNAL")
			ipv6ipv4.AddCustomNetworkWithStackType(network2, subnetwork2, "IPV4_ONLY", "")

			ipv6dual.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV6_ONLY", "EXTERNAL")
			ipv6dual.AddCustomNetworkWithStackType(network2, subnetwork2, "IPV4_IPV6", "INTERNAL")

			ipv6ipv6.AddCustomNetworkWithStackType(network1, subnetwork1, "IPV6_ONLY", "EXTERNAL")
			ipv6ipv6.AddCustomNetworkWithStackType(network2, subnetwork2, "IPV6_ONLY", "INTERNAL")

			allMultiVMs = append(allMultiVMs, ipv4dual, ipv4ipv6, dualipv4, dualdual, dualipv6, ipv6ipv4, ipv6dual, ipv6ipv6)
		}
		allVMs = append(allVMs, allMultiVMs...)
		for _, vm := range allMultiVMs {
			vm.AddScope("https://www.googleapis.com/auth/compute.readonly") // Readonly scope is needed for reading metadata.
		}
	}

	for _, vm := range allVMs {
		vm.AddMetadata(supportIpv6Key, strconv.FormatBool(supportsIpv6))
		vm.RunTests("TestNICSetup")
	}
	return nil
}
