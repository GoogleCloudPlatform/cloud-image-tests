// Copyright 2026 Google LLC
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

// Package networkutils contains utility functions for network-related operations.
package networkutils

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/compute-daisy"
	"google.golang.org/api/compute/v1"
)

const (
	// NICTypeVIRTIONET is the string type for a VIRTIO_NET interface.
	NICTypeVIRTIONET = "VIRTIO_NET"
	// NICTypeGVNIC is the string type for a GVNIC interface.
	NICTypeGVNIC = "GVNIC"
	// NICTypeIDPF is the string type for an IDPF interface.
	NICTypeIDPF = "IDPF"
	// NICTypeIRDMA is the string type for an IRDMA interface.
	NICTypeIRDMA = "IRDMA"
	// NICTypeMRDMA is the string type for an MRDMA interface.
	NICTypeMRDMA = "MRDMA"
)

var (
	// NICTypesFlag is the flag to specify the NIC types to use in the test.
	NICTypesFlag = flag.String("networkutils_nic_types", "GVNIC:1", "NIC types. Comma separated list of <NIC_TYPE>:<COUNT>. e.g. \"GVNIC:2\" or \"GVNIC:2,MRDMA:8\". If unspecified, defaults to a single GVNIC.")

	// ProvisioningModelFlag is the flag to specify the provisioning model to use when creating
	// instances.
	ProvisioningModelFlag = flag.String("networkutils_provisioning_model", "STANDARD", "Provisioning model to use when creating instances. One of [STANDARD, SPOT]. See https://docs.cloud.google.com/compute/docs/instances/provisioning-models.")

	// EthtoolDriverRe is a regex to extract the driver name from the `ethtool -i` output.
	EthtoolDriverRe = regexp.MustCompile(`(?m)^driver:\s*(.*)$`)

	nicTypeRegex = regexp.MustCompile(`^(.+):([0-9]+)$`)

	// ethtoolLRe is a regex to extract the max RX, TX, and Other queue counts from the
	// output of `ethtool -l`.
	ethtoolLRe = regexp.MustCompile(`(?m)Channel parameters for .*:\s*Pre-set maximums:\s*RX:\s*(\d+|n/a)\s*TX:\s*(\d+|n/a)\s*Other:\s*(\d+|n/a)\s*Combined:\s*(\d+|n/a)\s*Current hardware settings:\s*RX:\s*(\d+|n/a)\s*TX:\s*(\d+|n/a)\s*Other:\s*(\d+|n/a)\s*Combined:\s*(\d+|n/a)`)
)

// NetworkInterface represents a network interface in the Metadata Server.
type NetworkInterface struct {
	MAC     string `json:"mac"`
	NICType string `json:"nicType"`
}

// ListMDSIfaces returns a parsed list of network interfaces from the Metadata Server.
func ListMDSIfaces(ctx context.Context) ([]NetworkInterface, error) {
	networkInterfacesJSON, err := utils.GetRecursiveMetadata(ctx, "instance", "network-interfaces")
	if err != nil {
		return nil, err
	}

	var interfaces []NetworkInterface
	if err := json.Unmarshal([]byte(networkInterfacesJSON), &interfaces); err != nil {
		return nil, fmt.Errorf("unmarshalling network interfaces: %w", err)
	}

	return interfaces, nil
}

// Cpuset represents a set of CPUs on a system.
type Cpuset struct {
	cpus *big.Int
}

// ParseCpusetMask returns a Cpuset object parsed from a cpuset "Mask format" string.
// See https://man7.org/linux/man-pages/man7/cpuset.7.html#FORMATS for details.
func ParseCpusetMask(maskStr string) (*Cpuset, error) {
	maskStr = strings.TrimSpace(maskStr)
	maskStr = strings.ReplaceAll(maskStr, ",", "")

	if maskStr == "" {
		return &Cpuset{}, nil
	}

	cpus := big.NewInt(0)
	if _, ok := cpus.SetString(maskStr, 16); !ok {
		return nil, fmt.Errorf("failed to parse hex mask %q", maskStr)
	}

	return &Cpuset{cpus: cpus}, nil
}

// ParseCpusetList returns a Cpuset object parsed from a cpuset "List format" string.
// See https://man7.org/linux/man-pages/man7/cpuset.7.html#FORMATS for details.
func ParseCpusetList(listStr string) (*Cpuset, error) {
	if listStr == "" {
		return &Cpuset{}, nil
	}

	cpus := big.NewInt(0)
	ranges := strings.Split(listStr, ",")
	for _, cpuRange := range ranges {
		cpuRange = strings.TrimSpace(cpuRange)
		if cpuRange == "" {
			continue
		}

		var start, end int
		var err error
		if strings.Contains(cpuRange, "-") {
			cpuRangeParts := strings.Split(cpuRange, "-")
			if len(cpuRangeParts) != 2 {
				return nil, fmt.Errorf("failed to parse range %q", cpuRange)
			}
			start, err = strconv.Atoi(cpuRangeParts[0])
			if err != nil {
				return nil, fmt.Errorf("failed to parse range start %q: %v", cpuRangeParts[0], err)
			}
			end, err = strconv.Atoi(cpuRangeParts[1])
			if err != nil {
				return nil, fmt.Errorf("failed to parse range end %q: %v", cpuRangeParts[1], err)
			}
		} else {
			start, err = strconv.Atoi(cpuRange)
			if err != nil {
				return nil, fmt.Errorf("failed to parse cpu %q: %v", cpuRange, err)
			}
			end = start
		}

		if start > end {
			return nil, fmt.Errorf("non-monotonic range %q", cpuRange)
		}
		for cpu := start; cpu <= end; cpu++ {
			cpus.SetBit(cpus, cpu, 1)
		}
	}

	return &Cpuset{cpus: cpus}, nil
}

// ListString returns a cpuset "List format" as a string  from the given slice of integers.
// See https://man7.org/linux/man-pages/man7/cpuset.7.html#FORMATS for details.
func (c *Cpuset) ListString() string {
	if c.cpus == nil || c.cpus.BitLen() == 0 {
		return ""
	}

	var sortedCPUs []int
	for i := 0; i < c.cpus.BitLen(); i++ {
		if c.cpus.Bit(i) == 1 {
			sortedCPUs = append(sortedCPUs, i)
		}
	}

	type rng struct {
		start int
		end   int
	}
	ranges := []rng{{sortedCPUs[0], sortedCPUs[0]}}
	for i := 1; i < len(sortedCPUs); i++ {
		lastRange := &ranges[len(ranges)-1]
		if sortedCPUs[i] == lastRange.end+1 {
			lastRange.end = sortedCPUs[i]
			continue
		}
		ranges = append(ranges, rng{sortedCPUs[i], sortedCPUs[i]})
	}

	var result strings.Builder
	for i, r := range ranges {
		if i > 0 {
			result.WriteString(",")
		}
		if r.start == r.end {
			result.WriteString(strconv.Itoa(r.start))
		} else {
			result.WriteString(fmt.Sprintf("%d-%d", r.start, r.end))
		}
	}
	return result.String()
}

// ExpandNICTypes expands a comma separated list of <NIC_TYPE>:<COUNT> into a list of NIC types.
// e.g. "GVNIC:2,MRDMA:1" -> ["GVNIC", "GVNIC", "MRDMA"]
// If no NIC types are specified, defaults to a single GVNIC.
func ExpandNICTypes(condensedNicTypes string) ([]string, error) {
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
		nicTypes = append(nicTypes, NICTypeGVNIC)
	}

	return nicTypes, nil
}

func daisyNetworkForGeneralPurposeNIC(index int) *daisy.Network {
	return &daisy.Network{
		Network: compute.Network{
			Name: fmt.Sprintf("network-%d", index),
			Mtu:  int64(imagetest.JumboFramesMTU),
		},
		AutoCreateSubnetworks: new(bool),
	}
}

func daisyNetworkForIRDMANIC(index int, project string, zone string, isMetal bool) *daisy.Network {
	return &daisy.Network{
		Network: compute.Network{
			Name:           fmt.Sprintf("irdma-network-%d", index),
			Mtu:            int64(imagetest.JumboFramesMTU),
			NetworkProfile: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networkProfiles/%s-vpc-falcon", project, zone),
		},
		AutoCreateSubnetworks: new(bool),
	}
}

func daisyNetworkForMRDMANIC(index int, project string, zone string, isMetal bool) *daisy.Network {
	networkProfile := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networkProfiles/%s-vpc-roce", project, zone)
	if isMetal {
		networkProfile += "-metal"
	}
	return &daisy.Network{
		Network: compute.Network{
			Name:           fmt.Sprintf("mrdma-network-%d", index),
			Mtu:            int64(imagetest.JumboFramesMTU),
			NetworkProfile: networkProfile,
		},
		AutoCreateSubnetworks: new(bool),
	}
}

func daisyNetworkForNIC(nicType string, index int, project string, zone string, isMetal bool) (*daisy.Network, error) {
	switch nicType {
	case NICTypeVIRTIONET:
		return daisyNetworkForGeneralPurposeNIC(index), nil
	case NICTypeGVNIC:
		return daisyNetworkForGeneralPurposeNIC(index), nil
	case NICTypeIDPF:
		return daisyNetworkForGeneralPurposeNIC(index), nil
	case NICTypeIRDMA:
		return daisyNetworkForIRDMANIC(index, project, zone, isMetal), nil
	case NICTypeMRDMA:
		return daisyNetworkForMRDMANIC(index, project, zone, isMetal), nil
	default:
		return nil, fmt.Errorf("unsupported NIC type: %q", nicType)
	}
}

func subnetPrefix(index int) (string, error) {
	if index < 0 || index > 255 {
		return "", fmt.Errorf("index out of range [0, 255] is not supported, got %d", index)
	}
	return fmt.Sprintf("10.0.%d.0/24", index), nil
}

func regionFromZone(zone string) (string, error) {
	parts := strings.Split(zone, "-")
	if len(parts) < 2 {
		return "", fmt.Errorf("failed to parse region from zone %q", zone)
	}
	return strings.Join(parts[:2], "-"), nil
}

func daisySubnet(index int, zone string) (*daisy.Subnetwork, error) {
	netPrefix, err := subnetPrefix(index)
	if err != nil {
		return nil, err
	}

	region, err := regionFromZone(zone)
	if err != nil {
		return nil, err
	}

	return &daisy.Subnetwork{
		Subnetwork: compute.Subnetwork{
			Name:        fmt.Sprintf("subnet-%d", index),
			IpCidrRange: netPrefix,
			Region:      region,
		},
	}, nil
}

func accessConfigsForNIC(nicType string) []*compute.AccessConfig {
	if nicType == NICTypeIRDMA || nicType == NICTypeMRDMA {
		return []*compute.AccessConfig{}
	}
	return []*compute.AccessConfig{&compute.AccessConfig{
		Name: "External NAT",
		Type: "ONE_TO_ONE_NAT",
	}}
}

// CreateMachineWithNetworksOptions contains the options for creating a machine with multiple
// network interfaces.
type CreateMachineWithNetworksOptions struct {
	MachineName string
	MachineType string
	NicTypes    []string
	Project     string
	Zone        string
}

// CreateMachineWithNetworks creates a daisy instance with the given network interfaces.
// It registers the networks and subnetwork creations with the test workflow.
func CreateMachineWithNetworks(t *imagetest.TestWorkflow, o *CreateMachineWithNetworksOptions) (*daisy.Instance, error) {
	m := &daisy.Instance{}

	for nicIndex, nicType := range o.NicTypes {
		daisyNetwork, err := daisyNetworkForNIC(nicType, nicIndex, o.Project, o.Zone, imagetest.IsMetal(o.MachineType))
		if err != nil {
			return nil, fmt.Errorf("building daisy network: %w", err)
		}

		daisySubnet, err := daisySubnet(nicIndex, o.Zone)
		if err != nil {
			return nil, fmt.Errorf("building daisy subnetwork: %w", err)
		}

		citNetwork, err := t.CreateNetworkFromDaisyNetwork(daisyNetwork)
		if err != nil {
			return nil, fmt.Errorf("creating network: %w", err)
		}

		if _, err = citNetwork.CreateSubnetworkFromDaisySubnetwork(daisySubnet); err != nil {
			return nil, fmt.Errorf("creating subnetwork: %w", err)
		}

		m.NetworkInterfaces = append(m.NetworkInterfaces, &compute.NetworkInterface{
			NicType:       nicType,
			Network:       daisyNetwork.Name,
			Subnetwork:    daisySubnet.Name,
			AccessConfigs: accessConfigsForNIC(nicType),
		})
	}

	m.Scheduling = &compute.Scheduling{
		OnHostMaintenance: imagetest.MachineMaintenancePolicy(o.MachineType),
		ProvisioningModel: *ProvisioningModelFlag,
	}

	return m, nil
}

// EthtoolQueueCounts contains the queue counts for an interface as returned by `ethtool -l`.
// Fields should be set to -1 if the value is "n/a".
type EthtoolQueueCounts struct {
	MaxRXQueues       int
	MaxTXQueues       int
	MaxOtherQueues    int
	MaxCombinedQueues int

	CurrentTXQueues       int
	CurrentRXQueues       int
	CurrentOtherQueues    int
	CurrentCombinedQueues int
}

// ParseEthtoolLOutput parses the output of `ethtool -l` and returns an EthtoolQueueCounts
// object. Populates fields with -1 if the value is "n/a", returns an error if the output
// cannot be parsed.
func ParseEthtoolLOutput(output string) (*EthtoolQueueCounts, error) {
	matches := ethtoolLRe.FindStringSubmatch(output)
	if len(matches) != 9 {
		return nil, fmt.Errorf("parsing ethtool -l output: expected 9 matches, got %d", len(matches))
	}

	parseQueueCount := func(s string) (int, error) {
		if s == "n/a" {
			return -1, nil
		}
		return strconv.Atoi(s)
	}

	maxRXQueues, err := parseQueueCount(matches[1])
	if err != nil {
		return nil, fmt.Errorf("parsing max RX queues: %v", err)
	}
	maxTXQueues, err := parseQueueCount(matches[2])
	if err != nil {
		return nil, fmt.Errorf("parsing max TX queues: %v", err)
	}
	maxOtherQueues, err := parseQueueCount(matches[3])
	if err != nil {
		return nil, fmt.Errorf("parsing max Other queues: %v", err)
	}
	maxCombinedQueues, err := parseQueueCount(matches[4])
	if err != nil {
		return nil, fmt.Errorf("parsing max Combined queues: %v", err)
	}
	currentRXQueues, err := parseQueueCount(matches[5])
	if err != nil {
		return nil, fmt.Errorf("parsing current RX queues: %v", err)
	}
	currentTXQueues, err := parseQueueCount(matches[6])
	if err != nil {
		return nil, fmt.Errorf("parsing current TX queues: %v", err)
	}
	currentOtherQueues, err := parseQueueCount(matches[7])
	if err != nil {
		return nil, fmt.Errorf("parsing current Other queues: %v", err)
	}
	currentCombinedQueues, err := parseQueueCount(matches[8])
	if err != nil {
		return nil, fmt.Errorf("parsing current Combined queues: %v", err)
	}

	return &EthtoolQueueCounts{
		MaxRXQueues:           maxRXQueues,
		MaxTXQueues:           maxTXQueues,
		MaxOtherQueues:        maxOtherQueues,
		MaxCombinedQueues:     maxCombinedQueues,
		CurrentRXQueues:       currentRXQueues,
		CurrentTXQueues:       currentTXQueues,
		CurrentOtherQueues:    currentOtherQueues,
		CurrentCombinedQueues: currentCombinedQueues,
	}, nil
}
