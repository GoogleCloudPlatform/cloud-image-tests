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

// Package network is a CIT suite for testing network configuration functionality.
package network

import (
    "flag"
    "fmt"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

// Name is the name of the test package. It must match the directory name.
var Name = "network"

var testExcludeFilter = flag.String("network_test_exclude_filter", "", "Regex filter that excludes network test cases. Only cases with a matching test name will be skipped.")

// InstanceConfig for setting up test VMs.
type InstanceConfig struct {
	name string
	ip   string
}

var vm1Config = InstanceConfig{name: "ping1", ip: "192.168.0.2"}
var vm2Config = InstanceConfig{name: "ping2", ip: "192.168.0.3"}

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	exfilter, err := regexp.Compile(*testExcludeFilter)
	if err != nil {
		return fmt.Errorf("Invalid test case exclude filter: %v", err)
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
	if err := network1.CreateFirewallRule("allow-tcp-net1", "tcp", nil, []string{"10.128.0.0/20"}); err != nil {
		return err
	}

	network2, err := t.CreateNetwork("network-2", false)
	if err != nil {
		return err
	}
	subnetwork2, err := network2.CreateSubnetwork("subnetwork-2", "192.168.0.0/16")
	if err != nil {
		return err
	}
	if err := network2.CreateFirewallRule("allow-tcp-net2", "tcp", nil, []string{"192.168.0.0/16"}); err != nil {
		return err
	}

	vm1, err := t.CreateTestVM(vm1Config.name)
	if err != nil {
		return err
	}
	if err := vm1.AddCustomNetwork(network1, subnetwork1); err != nil {
		return err
	}
	if err := vm1.AddCustomNetwork(network2, subnetwork2); err != nil {
		return err
	}
	if err := vm1.SetPrivateIP(network2, vm1Config.ip); err != nil {
		return err
	}
    if exfilter.MatchString("TestSendPing") {
        fmt.Println("Skipping test 'TestSendPing'")
    } else {
	    vm1.RunTests("TestSendPing")
	}
	if exfilter.MatchString("TestDHCP") {
        fmt.Println("Skipping test 'TestDHCP'")
    } else {
	    vm1.RunTests("TestDHCP")
	}
	if exfilter.MatchString("TestDefaultMTU") {
        fmt.Println("Skipping test 'TestDefaultMTU'")
    } else {
	    vm1.RunTests("TestDefaultMTU")
	}
	if exfilter.MatchString("TestNTP") {
        fmt.Println("Skipping test 'TestNTP'")
    } else {
	    vm1.RunTests("TestNTP")
	}

	multinictests := "TestStaticIP|TestWaitForPing"
	if exfilter.MatchString("TestStaticIP") {
        fmt.Println("Skipping test 'TestStaticIP'")
        multinictests = strings.ReplaceAll(multinictests, "TestStaticIP", "")
    }
    if exfilter.MatchString("TestWaitForPing") {
        fmt.Println("Skipping test 'TestWaitForPing'")
        multinictests = strings.ReplaceAll(multinictests, "|TestWaitForPing", "")
    }
	if !utils.HasFeature(t.Image, "WINDOWS") && !strings.Contains(t.Image.Name, "sles-15") && !strings.Contains(t.Image.Name, "opensuse-leap") && !strings.Contains(t.Image.Name, "ubuntu-1604") && !strings.Contains(t.Image.Name, "ubuntu-pro-1604") && !strings.Contains(t.Image.Name, "cos") && !exfilter.MatchString("TestAlias") {
		multinictests += "|TestAlias"
	}

	// VM2 for multiNIC
	networkRebootInst := &daisy.Instance{}
	networkRebootInst.Metadata = map[string]string{imagetest.ShouldRebootDuringTest: "true"}
	vm2, err := t.CreateTestVMMultipleDisks([]*compute.Disk{{Name: vm2Config.name}}, networkRebootInst)
	if err != nil {
		return err
	}
	vm2.AddMetadata("enable-guest-attributes", "TRUE")
	if err := vm2.AddCustomNetwork(network1, subnetwork1); err != nil {
		return err
	}
	if err := vm2.AddCustomNetwork(network2, subnetwork2); err != nil {
		return err
	}
	if err := vm2.SetPrivateIP(network2, vm2Config.ip); err != nil {
		return err
	}
	if err := vm2.AddAliasIPRanges("10.14.8.0/24", "secondary-range"); err != nil {
		return err
	}
	if err := vm2.Reboot(); err != nil {
		return err
	}
	el7Re := regexp.MustCompile(`(centos|rhel)-7`)
	if utils.HasFeature(t.Image, "GVNIC") && !el7Re.MatchString(t.Image.Family) && !exfilter.MatchString("TestGVNIC") {
		multinictests += "|TestGVNIC"
		vm2.UseGVNIC()
	}
    if len(multinictests) > 0 {
	    vm2.RunTests(multinictests)
	}

	if el7Re.MatchString(t.Image.Family) {
		if exfilter.MatchString("TestGVNIC") {
            fmt.Println("Skipping test 'TestGVNIC'")
        } else {
            vm3, err := t.CreateTestVM("testGVNICEl7")
            if err != nil {
                return err
            }
            vm3.RunTests("TestGVNIC")
            vm3.UseGVNIC()
	    }
	}

	return nil
}
