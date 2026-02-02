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
	"strconv"
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
	nicTypes                = flag.String("networkperf_nic_types", "", "NIC types. Comma separated list of <NIC_TYPE>:<COUNT>. e.g. \"GVNIC:2\" or \"GVNIC:2,MRDMA:8\". If unspecified, defaults to a single GVNIC.")

	nicTypeRegex = regexp.MustCompile(`^(.+):([0-9]+)$`)
)

type networkTier string

const (
	defaultTier networkTier = "DEFAULT"
	tier1Tier   networkTier = "TIER_1"

	x86_64 string = "X86_64"
	arm64  string = "ARM64"

	nicTypeGVNIC string = "GVNIC"
)

// networkPerfConfig is a collection of related test configuration data
// which expands into individual networkPerfTest test cases.
type networkPerfConfig struct {
	machineType string        // Machine Type used for test
	arch        string        // CPU architecture for the machine type.
	networks    []networkTier // Network tiers to test.
	zone        string        // (optional) zone required for machine type.
	nicTypes    string        // (optional) NIC types for the machine. Defaults to a single GVNIC if unspecified. Follows the format of the flag --networkperf_nic_types.
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
	nicTypes    []string
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
		zone:        "us-central1-a",
	},
	{
		machineType: "c4-standard-192",
		arch:        x86_64,
		networks:    []networkTier{defaultTier, tier1Tier},
		zone:        "us-central1-a",
	},
}

func expandNetworkTestConfigs(configs []networkPerfConfig) ([]networkPerfTest, error) {
	var tests []networkPerfTest
	for _, config := range configs {
		for _, mtu := range []int{imagetest.DefaultMTU, imagetest.JumboFramesMTU} {
			for _, network := range config.networks {
				nicTypes, err := expandNICTypes(config.nicTypes)
				if err != nil {
					return nil, err
				}

				test := networkPerfTest{
					name:        fmt.Sprintf("%s_%d_%s", config.machineType, mtu, network),
					machineType: config.machineType,
					zone:        config.zone,
					arch:        config.arch,
					network:     network,
					mtu:         mtu,
					nicTypes:    nicTypes,
				}
				tests = append(tests, test)
			}
		}
	}
	return tests, nil
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
		return 0, fmt.Errorf("unknown network tier: %q", networkTier)
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

type createNetworkOptions struct {
	namePrefix string
	region     string
	mtu        int
	nicTypes   []string
}

func createNetworks(t *imagetest.TestWorkflow, opts *createNetworkOptions) ([]*networkConfig, error) {
	var networkConfigs []*networkConfig

	for ifaceIndex, nicType := range opts.nicTypes {
		if nicType != "GVNIC" {
			return nil, fmt.Errorf("unsupported nic type: %q", nicType)
		}

		ifNameprefix := fmt.Sprintf("%s%d", opts.namePrefix, ifaceIndex)

		network, err := t.CreateNetwork(ifNameprefix, false)
		if err != nil {
			return nil, err
		}
		subnetwork, err := network.CreateSubnetwork(ifNameprefix, networkPrefix(ifaceIndex))
		if err != nil {
			return nil, err
		}
		subnetwork.SetRegion(opts.region)
		if err := network.CreateFirewallRule("allow-iperf-"+ifNameprefix, "tcp", []string{"5001-5010"}, []string{networkPrefix(ifaceIndex)}); err != nil {
			return nil, err
		}
		if opts.mtu == imagetest.JumboFramesMTU {
			network.SetMTU(imagetest.JumboFramesMTU)
		}

		networkConfigs = append(networkConfigs, &networkConfig{
			network:    network,
			subnetwork: subnetwork,
			nicType:    nicType,
		})
	}

	return networkConfigs, nil
}

type networkConfig struct {
	network    *imagetest.Network
	subnetwork *imagetest.Subnetwork
	nicType    string
}

func createMachine(
	t *imagetest.TestWorkflow,
	testName string,
	machinePrefix string,
	networkConfigs []*networkConfig,
	machineType string,
	zone string,
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
	if *useSpotInstances {
		instance.Scheduling = &compute.Scheduling{
			Preemptible: true,
		}
	}

	vm, err := t.CreateTestVMMultipleDisks([]*compute.Disk{&disk}, instance)
	if err != nil {
		return nil, fmt.Errorf("creating machine: %w", err)
	}
	vm.ForceMachineType(machineType)
	vm.ForceZone(zone)
	for ifaceIndex, networkConfig := range networkConfigs {
		var address string
		if machinePrefix == "client" {
			address = clientAddress(ifaceIndex)
		} else if machinePrefix == "server" {
			address = serverAddress(ifaceIndex)
		} else {
			return nil, fmt.Errorf("unknown machine prefix: %q", machinePrefix)
		}

		if err := vm.AddCustomNetwork(networkConfig.network, networkConfig.subnetwork); err != nil {
			return nil, fmt.Errorf("adding network: %w", err)
		}
		if err := vm.SetPrivateIP(networkConfig.network, address); err != nil {
			return nil, fmt.Errorf("setting private IP: %w", err)
		}
	}

	return vm, nil
}

func parseNetworkTiers(networkTiersStr string) ([]networkTier, error) {
	parts := strings.Split(networkTiersStr, ",")
	var tiers []networkTier
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch part {
		case string(defaultTier):
			tiers = append(tiers, defaultTier)
		case string(tier1Tier):
			tiers = append(tiers, tier1Tier)
		default:
			return nil, fmt.Errorf("invalid network tier: %q", part)
		}
	}
	return tiers, nil
}

func testConfigs(t *imagetest.TestWorkflow, filter *regexp.Regexp) ([]networkPerfTest, error) {
	var networkPerfTests []networkPerfTest
	var err error

	if *useMachineParamsFromCLI {
		// Flag-driven workflow for running a specific machine only.

		networkTiers, err := parseNetworkTiers(*networkTiers)
		if err != nil {
			return nil, fmt.Errorf("parsing network tiers: %w", err)
		}
		citNetworkPerfTestConfig := networkPerfConfig{
			machineType: t.MachineType.Name,
			zone:        t.Zone.Name,
			arch:        x86_64,
			networks:    networkTiers,
			nicTypes:    *nicTypes,
		}
		networkPerfTests, err = expandNetworkTestConfigs([]networkPerfConfig{citNetworkPerfTestConfig})
		if err != nil {
			return nil, fmt.Errorf("expanding network test configs: %w", err)
		}
		return networkPerfTests, nil
	}

	// Default workflow for running all configured machine types.
	networkPerfTests, err = expandNetworkTestConfigs(defaultNetworkPerfTestConfigs)
	if err != nil {
		return nil, fmt.Errorf("expanding network test configs: %w", err)
	}
	networkPerfTests = filterNetworkTestConfigs(networkPerfTests, filter, t.Image.Architecture)

	return networkPerfTests, nil
}

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	filter, err := regexp.Compile(*testFilter)
	if err != nil {
		return fmt.Errorf("invalid networkperf test filter: %v", err)
	}
	if !utils.HasFeature(t.Image, "GVNIC") {
		t.Skip(fmt.Sprintf("%q does not support GVNIC", t.Image.Name))
	}

	networkPerfTests, err := testConfigs(t, filter)
	if err != nil {
		return fmt.Errorf("getting test configs: %w", err)
	}

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

		networkOpts := &createNetworkOptions{
			namePrefix: sanitizeResourceName(tc.name),
			region:     region,
			mtu:        tc.mtu,
			nicTypes:   tc.nicTypes,
		}
		networkConfigs, err := createNetworks(t, networkOpts)
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
			networkConfigs,
			tc.machineType,
			zone,
		)
		if err != nil {
			return fmt.Errorf("creating server machine: %w", err)
		}
		clientVM, err := createMachine(
			t,
			tc.name,
			"client",
			networkConfigs,
			tc.machineType,
			zone,
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
		clientVM.AddMetadata("num-parallel-tests", fmt.Sprint(len(networkConfigs)))
		serverVM.AddMetadata("num-parallel-tests", fmt.Sprint(len(networkConfigs)))
		for i := range networkConfigs {
			clientVM.AddMetadata("iperftarget-"+fmt.Sprint(i), serverAddress(i))
		}
		clientVM.AddMetadata("expectedperf", fmt.Sprint(perfTarget))
		clientVM.AddMetadata("network-tier", string(tc.network))

		clientVM.UseGVNIC()
		serverVM.UseGVNIC()

		serverVM.RunTests("TestGVNICExists")
		clientVM.RunTests("TestGVNICExists|TestNetworkPerformance")
	}
	return nil
}

// expandNICTypes expands a comma separated list of <NIC_TYPE>:<COUNT> into a list of NIC types.
// e.g. "GVNIC:2,MRDMA:1" -> ["GVNIC", "GVNIC", "MRDMA"]
// If no NIC types are specified, defaults to a single GVNIC.
func expandNICTypes(condensedNicTypes string) ([]string, error) {
	nicTypeCounts := strings.Split(condensedNicTypes, ",")
	var nicTypes []string
	for _, nicTypeCount := range nicTypeCounts {
		nicTypeCount = strings.TrimSpace(nicTypeCount)
		if nicTypeCount == "" {
			continue
		}
		matches := nicTypeRegex.FindStringSubmatch(nicTypeCount)
		if len(matches) != 3 {
			return nil, fmt.Errorf("invalid nic type count: %q", nicTypeCount)
		}
		nicType := matches[1]
		count, err := strconv.Atoi(matches[2])
		if err != nil {
			return nil, fmt.Errorf("invalid count: %v", err)
		}
		for i := 0; i < count; i++ {
			nicTypes = append(nicTypes, nicType)
		}
	}

	if len(nicTypes) == 0 {
		nicTypes = append(nicTypes, nicTypeGVNIC)
	}

	return nicTypes, nil
}

func networkPrefix(ifaceIndex int) string {
	return fmt.Sprintf("192.168.%d.0/24", ifaceIndex)
}

func clientAddress(ifaceIndex int) string {
	return fmt.Sprintf("192.168.%d.2", ifaceIndex)
}

func serverAddress(ifaceIndex int) string {
	return fmt.Sprintf("192.168.%d.3", ifaceIndex)
}
