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
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
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
	// EthtoolDriverRe is a regex to extract the driver name from the `ethtool -i` output.
	EthtoolDriverRe = regexp.MustCompile(`(?m)^driver:\s*(.*)$`)

	nicTypeRegex = regexp.MustCompile(`^(.+):([0-9]+)$`)
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
	cpus []int
}

// ParseCpusetMask returns a Cpuset object parsed from a cpuset "Mark format" string.
// See https://man7.org/linux/man-pages/man7/cpuset.7.html#FORMATS for details.
func ParseCpusetMask(maskStr string) (*Cpuset, error) {
	maskStr = strings.TrimSpace(maskStr)
	maskStr = strings.ReplaceAll(maskStr, ",", "")

	if maskStr == "" {
		return &Cpuset{}, nil
	}

	var i big.Int
	if _, ok := i.SetString(maskStr, 16); !ok {
		return nil, fmt.Errorf("failed to parse hex mask %q", maskStr)
	}

	var cpus []int
	for cpu := 0; cpu < i.BitLen(); cpu++ {
		if i.Bit(cpu) != 0 {
			cpus = append(cpus, cpu)
		}
	}
	return &Cpuset{cpus: cpus}, nil
}

// ListString returns a cpuset "List format" as a string  from the given slice of integers.
// See https://man7.org/linux/man-pages/man7/cpuset.7.html#FORMATS for details.
func (c *Cpuset) ListString() string {
	if len(c.cpus) == 0 {
		return ""
	}

	sortedCPUs := make([]int, len(c.cpus))
	copy(sortedCPUs, c.cpus)
	sort.Ints(sortedCPUs)

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
