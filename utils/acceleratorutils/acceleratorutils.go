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

// Package acceleratorutils provides common utility functions for accelerator tests.
package acceleratorutils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/compute-daisy"

	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
)

var (
	rdmaHostName           = "rdmahost"
	rdmaClientName         = "rdmaclient"
	gvnicNet0Name          = "gvnic-net0"
	gvnicNet0Sub0Name      = "gvnic-net0-sub0"
	gvnicNet1Name          = "gvnic-net1"
	gvnicNet1Sub0Name      = "gvnic-net1-sub0"
	mrdmaNetName           = "mrdma-net"
	firewallAllowProtocols = []string{"tcp", "udp", "icmp"}
)

// CreateNetwork creates the networks and subnetworks required for accelerator VMs.
func CreateNetwork(t *imagetest.TestWorkflow) ([]*computeBeta.NetworkInterface, error) {
	testZone := t.Zone.Name
	// For example, region should be us-central1 for zone us-central1-a.
	lastDashIndex := strings.LastIndex(testZone, "-")
	if lastDashIndex == -1 {
		return nil, fmt.Errorf("invalid zone: %s", testZone)
	}
	gvnicNet0, err := t.CreateNetwork(gvnicNet0Name, false)
	if err != nil {
		return nil, err
	}
	gvnicNet0Sub0, err := gvnicNet0.CreateSubnetwork(gvnicNet0Sub0Name, "192.168.0.0/24")
	if err != nil {
		return nil, err
	}
	for _, protocol := range firewallAllowProtocols {
		if err := gvnicNet0.CreateFirewallRule(gvnicNet0Name+"-allow-"+protocol, protocol, nil, []string{"192.168.0.0/24"}); err != nil {
			return nil, err
		}
	}
	testRegion := testZone[:lastDashIndex]
	gvnicNet0Sub0.SetRegion(testRegion)
	gvnicNet1, err := t.CreateNetwork(gvnicNet1Name, false)
	if err != nil {
		return nil, err
	}
	gvnicNet1Sub0, err := gvnicNet1.CreateSubnetwork(gvnicNet1Sub0Name, "192.168.1.0/24")
	if err != nil {
		return nil, err
	}
	for _, protocol := range firewallAllowProtocols {
		if err := gvnicNet1.CreateFirewallRule(gvnicNet1Name+"-allow-"+protocol, protocol, nil, []string{"192.168.1.0/24"}); err != nil {
			return nil, err
		}
	}
	gvnicNet1Sub0.SetRegion(testRegion)

	mrdma := &daisy.Network{}
	mrdma.Name = mrdmaNetName
	mrdma.Mtu = imagetest.JumboFramesMTU
	mrdma.AutoCreateSubnetworks = new(bool) // false
	mrdma.NetworkProfile = fmt.Sprintf("global/networkProfiles/%s-vpc-roce", testZone)
	mrdmaNet, err := t.CreateNetworkFromDaisyNetwork(mrdma)
	if err != nil {
		return nil, err
	}

	nicConfig := []*computeBeta.NetworkInterface{
		{
			NicType:    "GVNIC",
			Network:    gvnicNet0Name,
			Subnetwork: gvnicNet0Sub0Name,
		},
		{
			NicType:    "GVNIC",
			Network:    gvnicNet1Name,
			Subnetwork: gvnicNet1Sub0Name,
		},
	}
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("mrdma-net-sub-%d", i)
		mrdmaSubnet, err := mrdmaNet.CreateSubnetwork(name, fmt.Sprintf("192.168.%d.0/24", i+2))
		if err != nil {
			return nil, err
		}
		mrdmaSubnet.SetRegion(testRegion)
		// go/go-style/decisions#nil-slices
		// "Do not create APIs that force their clients to make distinctions
		// between nil and the empty slice."
		//
		// This is bad readability-wise, but we are using an API that makes
		// distinctions between nil and empty slices so not much choice.
		nicConfig = append(nicConfig, &computeBeta.NetworkInterface{
			NicType:       "MRDMA",
			Network:       mrdmaNetName,
			Subnetwork:    name,
			AccessConfigs: []*computeBeta.AccessConfig{},
		})
	}
	return nicConfig, nil
}

func createVM(t *imagetest.TestWorkflow, name string, nics []*computeBeta.NetworkInterface) (*imagetest.TestVM, error) {
	testZone := t.Zone.Name
	accelConfig := []*computeBeta.AcceleratorConfig{
		{
			AcceleratorCount: 8,
			AcceleratorType:  fmt.Sprintf("zones/%s/acceleratorTypes/%s", testZone, t.AcceleratorType),
		},
	}
	schedulingConfig := &computeBeta.Scheduling{OnHostMaintenance: "TERMINATE"}

	instance := &daisy.InstanceBeta{}
	instance.Name = name
	instance.MachineType = t.MachineType.Name
	instance.Zone = testZone
	instance.Scheduling = schedulingConfig
	instance.NetworkInterfaces = nics
	instance.GuestAccelerators = accelConfig

	disks := []*compute.Disk{{Name: name, Type: imagetest.HyperdiskBalanced, Zone: testZone, SizeGb: 80}}

	return t.CreateTestVMFromInstanceBeta(instance, disks)
}

// CreateHostAndClientVMs creates a host and client VM for 2 node accelerator tests.
func CreateHostAndClientVMs(t *imagetest.TestWorkflow, nics []*computeBeta.NetworkInterface) (*imagetest.TestVM, *imagetest.TestVM, error) {
	hostVM, err := createVM(t, rdmaHostName, nics)
	if err != nil {
		return nil, nil, err
	}
	clientVM, err := createVM(t, rdmaClientName, nics)
	if err != nil {
		return nil, nil, err
	}
	return hostVM, clientVM, nil
}

// InstallIbVerbsUtils installs the ibverbs-utils package if it is not already part of the image.
func InstallIbVerbsUtils(ctx context.Context, t *testing.T) {
	t.Helper()
	// Rocky Linux images already have the tools from ibverbs-utils pre-installed.
	if IsRockyLinux(ctx, t) {
		return
	}
	if out, err := exec.CommandContext(ctx, "apt", "update").CombinedOutput(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, apt update).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", err, out)
	}
	if out, err := exec.CommandContext(ctx, "apt", "install", "-y", "ibverbs-utils").CombinedOutput(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, apt install, -y, ibverbs-utils).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", err, out)
	}
}

// IsRockyLinux checks if the current OS is Rocky Linux.
func IsRockyLinux(ctx context.Context, t *testing.T) bool {
	t.Helper()
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		t.Logf("Could not read /etc/os-release: %v, defaulting IsRockyLinux to false", err)
		return false
	}
	return strings.Contains(string(content), "rocky")
}

// RunRDMAClientCommand executes a RDMA test command targeting the host VM. The host VM must run the
// same command. It retries on connection errors, as the client might be ready before the host.
func RunRDMAClientCommand(ctx context.Context, t *testing.T, command string, args []string) {
	t.Helper()
	target, err := utils.GetRealVMName(ctx, rdmaHostName)
	if err != nil {
		t.Fatalf("utils.GetRealVMName(%s) = %v, want nil", rdmaHostName, err)
	}
	fullArgs := append(args, target)
	for {
		command := exec.CommandContext(ctx, command, fullArgs...)
		out, err := command.CombinedOutput()
		if err == nil {
			t.Logf("%s output:\n%s", command, out)
			return
		}
		// Client may be ready before host, retry connection errors.
		if strings.Contains(string(out), "Couldn't connect to "+target) {
			time.Sleep(time.Second)
			if ctx.Err() != nil {
				t.Logf("%s output:\n%s", command, out)
				t.Fatalf("context expired before connecting to host: %v\nlast %q error was: %v", ctx.Err(), command, err)
			}
			continue
		}

		t.Logf("%s output:\n%s", command, out)
		t.Fatalf("exec.CommandContext(%s).CombinedOutput() failed unexpectedly; err %v", command, err)
	}
}
