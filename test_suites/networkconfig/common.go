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

package networkconfig

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
	nicTypeVIRTIONET = "VIRTIO_NET"
	nicTypeGVNIC     = "GVNIC"
	nicTypeIDPF      = "IDPF"
	nicTypeIRDMA     = "IRDMA"
	nicTypeMRDMA     = "MRDMA"
)

var ethtoolDriverRe = regexp.MustCompile(`(?m)^driver:\s*(.*)$`)

type networkInterface struct {
	MAC     string `json:"mac"`
	NICType string `json:"nicType"`
}

func listMDSIfaces(ctx context.Context) ([]networkInterface, error) {
	networkInterfacesJSON, err := utils.GetRecursiveMetadata(ctx, "instance", "network-interfaces")
	if err != nil {
		return nil, err
	}

	var interfaces []networkInterface
	if err := json.Unmarshal([]byte(networkInterfacesJSON), &interfaces); err != nil {
		return nil, fmt.Errorf("unmarshalling network interfaces: %w", err)
	}

	return interfaces, nil
}

// ARNTODO these need to move to some utilities file.
func parseHexMask(maskStr string) ([]int, error) {
	maskStr = strings.TrimSpace(maskStr)
	maskStr = strings.ReplaceAll(maskStr, ",", "")

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
	return cpus, nil
}

// cpuListString returns a cpuset "List Representation" as a string
// from the given slice of integers.
// See https://man7.org/linux/man-pages/man7/cpuset.7.html for details.
func cpuListString(cpus []int) string {
	if len(cpus) == 0 {
		return ""
	}
	sortedCPUs := make([]int, len(cpus))
	copy(sortedCPUs, cpus)
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
