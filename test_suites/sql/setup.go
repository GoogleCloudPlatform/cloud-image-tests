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

// Package sql tests windows SQL server functionality.
package sql

import (
	"embed"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Name is the name of the test package. It must match the directory name.
var Name = "sql"

// InstanceConfig for setting up test VMs.
type InstanceConfig struct {
	name string
	ip   string
}

var serverConfig = InstanceConfig{name: "server", ip: "192.168.0.10"}
var clientConfig = InstanceConfig{name: "client", ip: "192.168.0.11"}

//go:embed *
var scripts embed.FS

const (
	serverStartupScriptURL = "startupscripts/remote_auth_server_setup.ps1"
	clientStartupScriptURL = "startupscripts/remote_auth_client_setup.ps1"
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if utils.HasFeature(t.Image, "WINDOWS") && strings.Contains(t.Image.Name, "sql") {
		defaultNetwork, err := t.CreateNetwork("default-network", false)
		if err != nil {
			return err
		}
		defaultSubnetwork, err := defaultNetwork.CreateSubnetwork("default-subnetwork", "192.168.0.0/24")
		if err != nil {
			return err
		}
		if err := defaultNetwork.CreateFirewallRule("allow-sql-tcp", "tcp", []string{"135", "1433", "1434", "4022", "5022"}, []string{"192.168.0.0/24"}); err != nil {
			return err
		}
		if err := defaultNetwork.CreateFirewallRule("allow-sql-udp", "udp", []string{"1434"}, []string{"192.168.0.0/24"}); err != nil {
			return err
		}

		// Get the startup scripts as byte arrays.
		serverStartupByteArr, err := scripts.ReadFile(serverStartupScriptURL)
		if err != nil {
			return err
		}
		clientStartupByteArr, err := scripts.ReadFile(clientStartupScriptURL)
		if err != nil {
			return err
		}
		serverStartup := string(serverStartupByteArr)
		clientStartup := string(clientStartupByteArr)

		serverVM, err := t.CreateTestVM(serverConfig.name)
		if err != nil {
			return err
		}
		if err := serverVM.AddCustomNetwork(defaultNetwork, defaultSubnetwork); err != nil {
			return err
		}
		if err := serverVM.SetPrivateIP(defaultNetwork, serverConfig.ip); err != nil {
			return err
		}

		clientVM, err := t.CreateTestVM(clientConfig.name)
		if err != nil {
			return err
		}
		if err := clientVM.AddCustomNetwork(defaultNetwork, defaultSubnetwork); err != nil {
			return err
		}
		if err := clientVM.SetPrivateIP(defaultNetwork, clientConfig.ip); err != nil {
			return err
		}
		clientVM.AddMetadata("enable-guest-attributes", "TRUE")
		clientVM.AddMetadata("sqltarget", serverConfig.ip)

		serverVM.AddMetadata("windows-startup-script-ps1", serverStartup)
		clientVM.AddMetadata("windows-startup-script-ps1", clientStartup)

		vm1, err := t.CreateTestVM("settings")
		if err != nil {
			return err
		}
		vm1.RunTests("TestSqlVersion|TestPowerPlan")
		clientVM.RunTests("TestRemoteConnectivity")
		serverVM.RunTests("TestPowerPlan")
	}
	return nil
}
