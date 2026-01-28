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

// Package networkperf is a CIT suite for testing that network performance
// reaches expected targets.
package networkperf

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisy "github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

var (
	// Name is the name of the test package. It must match the directory name.
	Name = "networkperf"

	testFilter              = flag.String("networkperf_test_filter", ".*", "regexp filter for networkperf test cases, only cases with a matching name will be run")
	useSpotInstances        = flag.Bool("networkperf_use_spot_instances", false, "use spot instances for networkperf test cases")
	useMachineParamsFromCLI = flag.Bool("networkperf_machine_params_from_cli", false, "use the machine parameters (machine type, zone) provided by the CIT wrapper instead of the default machine parameters built into the test")
	networkTiers            = flag.String("networkperf_network_tiers", "", "comma separated list of network tiers to test (DEFAULT|TIER_1)")

	networkPrefix = "192.168.0.0/24"
	clientAddress = "192.168.0.2"
	serverAddress = "192.168.0.3"
)

type networkTier string

const (
	defaultTier networkTier = "DEFAULT"
	tier1Tier   networkTier = "TIER_1"

	x86_64 string = "X86_64"
	arm64  string = "ARM64"
)

// networkPerfConfig is a collection of related test configuration data
// which expands into individual networkPerfTest test cases.
type networkPerfConfig struct {
	machineType string        // Machine Type used for test
	arch        string        // CPU architecture for the machine type.
	networks    []networkTier // Network tiers to test.
	zone        string        // (optional) zone required for machine type.
}

// networkPerfTest is a single test case measuring network performance for
// particular configurations.
type networkPerfTest struct {
	name        string
	machineType string
	zone        string
	arch        string
	network     networkTier
	mtu         int
}

var defaultNetworkPerfTestConfigs = []networkPerfConfig{
	{
		machineType: "n1-standard-2",
		arch:        x86_64,
		networks:    []networkTier{defaultTier},
	},
	{
		machineType: "n2-standard-2",
		arch:        x86_64,
		networks:    []networkTier{defaultTier},
	},
	{
		machineType: "n2d-standard-2",
		arch:        x86_64,
		networks:    []networkTier{defaultTier},
	},
	{
		machineType: "e2-standard-2",
		arch:        x86_64,
		networks:    []networkTier{defaultTier},
	},
	{
		machineType: "t2d-standard-1",
		arch:        x86_64,
		networks:    []networkTier{defaultTier},
	},
	{
		machineType: "t2a-standard-1",
		arch:        arm64,
		networks:    []networkTier{defaultTier},
		zone:        "us-central1-a",
	},
	{
		machineType: "n2-standard-32",
		arch:        x86_64,
		networks:    []networkTier{defaultTier, tier1Tier},
	},
	{
		machineType: "n2d-standard-48",
		arch:        x86_64,
		networks:    []networkTier{defaultTier, tier1Tier},
	},
	{
		machineType: "n4-standard-16",
		arch:        x86_64,
		networks:    []networkTier{defaultTier},
		zone:        "us-central1-b",
	},
	{
		machineType: "n4-standard-80",
		arch:        x86_64,
		networks:    []networkTier{defaultTier},
		zone:        "us-central1-b",
	},
	{
		machineType: "c4-standard-2",
		arch:        x86_64,
		networks:    []networkTier{defaultTier},
		zone:        "us-east4-b",
	},
	{
		machineType: "c4-standard-192",
		arch:        x86_64,
		networks:    []networkTier{defaultTier, tier1Tier},
		zone:        "us-east4-b",
	},
}

func expandNetworkTestConfigs(configs []networkPerfConfig) []networkPerfTest {
	var tests []networkPerfTest
	for _, config := range configs {
		for _, mtu := range []int{imagetest.DefaultMTU, imagetest.JumboFramesMTU} {
			for _, network := range config.networks {

				test := networkPerfTest{
					name:        fmt.Sprintf("%s_%d_%s", config.machineType, mtu, network),
					machineType: config.machineType,
					zone:        config.zone,
					arch:        config.arch,
					network:     network,
					mtu:         mtu,
				}
				tests = append(tests, test)
			}
		}
	}
	return tests
}

func filterNetworkTestConfigs(
	tests []networkPerfTest,
	filter *regexp.Regexp,
	imageArch string,
) []networkPerfTest {
	var filtered []networkPerfTest
	for _, test := range tests {
		if !filter.MatchString(test.name) {
			continue
		}
		if test.arch != imageArch {
			continue
		}

		filtered = append(filtered, test)
	}
	return filtered
}

//go:embed startupscripts/*
var scripts embed.FS

//go:embed targets/*
var targets embed.FS

const (
	linuxInstallStartupScriptURI = "startupscripts/linux_common.sh"
	linuxServerStartupScriptURI  = "startupscripts/linux_serverstartup.sh"
	linuxClientStartupScriptURI  = "startupscripts/linux_clientstartup.sh"

	windowsInstallStartupScriptURI = "startupscripts/windows_common.ps1"
	windowsServerStartupScriptURI  = "startupscripts/windows_serverstartup.ps1"
	windowsClientStartupScriptURI  = "startupscripts/windows_clientstartup.ps1"

	targetsURL      = "targets/default_targets.txt"
	tier1TargetsURL = "targets/tier1_targets.txt"
)

// getExpectedPerf gets the expected performance of the given machine type. Since the targets map only contains breakpoints in vCPUs at which
// each machine type's expected performance changes, find the highest breakpoint at which the expected performance would change, then return
// the performance at said breakpoint.
func getExpectedPerf(targetMap map[string]int, machineType *compute.MachineType) (int, error) {
	// Return if already at breakpoint.
	perf, found := targetMap[machineType.Name]
	if found {
		return perf, nil
	}

	numCPUs := machineType.GuestCpus

	// Decrement numCPUs until a breakpoint is found.
	for !found {
		numCPUs--
		perf, found = targetMap[regexp.MustCompile("-[0-9]+$").ReplaceAllString(machineType.Name, fmt.Sprintf("-%d", numCPUs))]
		if !found && numCPUs <= 1 {
			return 0, fmt.Errorf("Error: appropriate perf target not found for %v", machineType)
		}
	}
	return perf, nil
}

func perfTarget(machineType *compute.MachineType, networkTier networkTier) (int, error) {
	var targetsFile string
	switch networkTier {
	case defaultTier:
		targetsFile = targetsURL
	case tier1Tier:
		targetsFile = tier1TargetsURL
	default:
		return 0, fmt.Errorf("unknown network tier: %s", networkTier)
	}

	var perfTargets map[string]int
	perfTargetsString, err := targets.ReadFile(targetsFile)
	if err != nil {
		return 0, err
	}
	if err := json.Unmarshal(perfTargetsString, &perfTargets); err != nil {
		return 0, err
	}
	perfTarget, err := getExpectedPerf(perfTargets, machineType)
	if err != nil {
		return 0, fmt.Errorf("could not get default perf target: %v", err)
	}

	return perfTarget, nil
}

func startupScripts(image *compute.Image) (string, string, error) {
	var startupURIs struct {
		common string
		client string
		server string
	}
	if utils.HasFeature(image, "WINDOWS") {
		startupURIs.common = windowsInstallStartupScriptURI
		startupURIs.client = windowsClientStartupScriptURI
		startupURIs.server = windowsServerStartupScriptURI
	} else {
		startupURIs.common = linuxInstallStartupScriptURI
		startupURIs.client = linuxClientStartupScriptURI
		startupURIs.server = linuxServerStartupScriptURI
	}

	commonStartupBytes, err := scripts.ReadFile(startupURIs.common)
	if err != nil {
		return "", "", err
	}
	serverStartupBytes, err := scripts.ReadFile(startupURIs.server)
	if err != nil {
		return "", "", err
	}
	clientStartupBytes, err := scripts.ReadFile(startupURIs.client)
	if err != nil {
		return "", "", err
	}

	serverStartup := string(commonStartupBytes) + string(serverStartupBytes)
	clientStartup := string(commonStartupBytes) + string(clientStartupBytes)

	return serverStartup, clientStartup, nil
}

func sanitizeResourceName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "-", "")
	return name
}

func createNetwork(t *imagetest.TestWorkflow, testName string, machineType string, region string, mtu int) (*imagetest.Network, *imagetest.Subnetwork, error) {
	safeName := sanitizeResourceName(testName)
	network, err := t.CreateNetwork(safeName, false)
	if err != nil {
		return nil, nil, err
	}
	subnetwork, err := network.CreateSubnetwork(safeName, networkPrefix)
	if err != nil {
		return nil, nil, err
	}
	subnetwork.SetRegion(region)
	if err := network.CreateFirewallRule("allow-iperf-"+safeName, "tcp", []string{"5001-5010"}, []string{networkPrefix}); err != nil {
		return nil, nil, err
	}
	if mtu == imagetest.JumboFramesMTU {
		network.SetMTU(imagetest.JumboFramesMTU)
	}

	return network, subnetwork, nil
}

func createMachine(
	t *imagetest.TestWorkflow,
	testName string,
	machinePrefix string,
	network *imagetest.Network,
	subnetwork *imagetest.Subnetwork,
	machineType string,
	zone string,
	networkAddress string,
) (*imagetest.TestVM, error) {

	// "Dashes are disallowed in testworkflow vm names".
	// And GCE doesn't allow underscores. So get rid of both.
	name := fmt.Sprintf("%s%s", machinePrefix, sanitizeResourceName(testName))
	disk := compute.Disk{
		Name: name,
		Type: imagetest.DiskTypeNeeded(machineType),
		Zone: zone,
	}

	instance := &daisy.Instance{}
	instance.Scheduling = &compute.Scheduling{
		Preemptible: true,
	}

	vm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{&disk}, instance)
	if err != nil {
		return nil, fmt.Errorf("creating machine: %w", err)
	}
	vm.ForceMachineType(machineType)
	vm.ForceZone(zone)
	if err := vm.AddCustomNetwork(network, subnetwork); err != nil {
		return nil, fmt.Errorf("adding network: %w", err)
	}
	if err := vm.SetPrivateIP(network, networkAddress); err != nil {
		return nil, fmt.Errorf("setting private IP: %w", err)
	}

	return vm, nil
}

func testConfigs(t *imagetest.TestWorkflow, filter *regexp.Regexp) []networkPerfTest {
	var networkPerfTests []networkPerfTest

	if *useMachineParamsFromCLI {
		// Flag-driven workflow for running a specific machine only.
		citNetworkPerfTestConfig := networkPerfConfig{
			machineType: t.MachineType.Name,
			zone:        t.Zone.Name,
			arch:        x86_64,
			networks:    []networkTier{defaultTier},
		}
		networkPerfTests = expandNetworkTestConfigs([]networkPerfConfig{citNetworkPerfTestConfig})
	} else {
		// Default workflow for running all configured machine types.
		networkPerfTests = expandNetworkTestConfigs(defaultNetworkPerfTestConfigs)
		networkPerfTests = filterNetworkTestConfigs(networkPerfTests, filter, t.Image.Architecture)
	}

	return networkPerfTests
}

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	filter, err := regexp.Compile(*testFilter)
	if err != nil {
		return fmt.Errorf("invalid networkperf test filter: %v", err)
	}
	if !utils.HasFeature(t.Image, "GVNIC") {
		t.Skip(fmt.Sprintf("%s does not support GVNIC", t.Image.Name))
	}

	networkPerfTests := testConfigs(t, filter)

	for _, tc := range networkPerfTests {
		var region string
		var zone string
		if tc.zone == "" {
			zone = t.Zone.Name
			region = path.Base(t.Zone.Region)
		} else {
			z, err := t.Client.GetZone(t.Project.Name, tc.zone)
			if err != nil {
				return err
			}
			zone = z.Name
			region = path.Base(z.Region)
		}
		machine, err := t.Client.GetMachineType(t.Project.Name, zone, tc.machineType)
		if err != nil {
			return err
		}

		network, subnetwork, err := createNetwork(t, tc.name, tc.machineType, region, tc.mtu)
		if err != nil {
			return fmt.Errorf("creating networks: %w", err)
		}

		serverStartup, clientStartup, err := startupScripts(t.Image)
		if err != nil {
			return fmt.Errorf("loading startup scripts: %w", err)
		}

		serverVM, err := createMachine(
			t,
			tc.name,
			"server",
			network,
			subnetwork,
			tc.machineType,
			zone,
			serverAddress,
		)
		if err != nil {
			return fmt.Errorf("creating server machine: %w", err)
		}
		clientVM, err := createMachine(
			t,
			tc.name,
			"client",
			network,
			subnetwork,
			tc.machineType,
			zone,
			clientAddress,
		)
		if err != nil {
			return fmt.Errorf("creating client machine: %w", err)
		}

		if utils.HasFeature(t.Image, "WINDOWS") {
			serverVM.SetWindowsStartupScript(serverStartup)
			clientVM.SetWindowsStartupScript(clientStartup)
		} else {
			serverVM.SetStartupScript(serverStartup)
			clientVM.SetStartupScript(clientStartup)
		}

		perfTarget, err := perfTarget(machine, tc.network)
		if err != nil {
			return fmt.Errorf("getting perf target: %w", err)
		}

		clientVM.AddMetadata("enable-guest-attributes", "TRUE")
		clientVM.AddMetadata("iperftarget", serverAddress)
		clientVM.AddMetadata("expectedperf", fmt.Sprint(perfTarget))
		clientVM.AddMetadata("network-tier", string(tc.network))

		clientVM.UseGVNIC()
		serverVM.UseGVNIC()

		serverVM.RunTests("TestGVNICExists")
		clientVM.RunTests("TestGVNICExists|TestNetworkPerformance")
	}
	return nil
}
