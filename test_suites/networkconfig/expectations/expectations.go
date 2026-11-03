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

// Package expectations evaluates guest OS constraints and normalizes network
// configuration expectations for comparison.
package expectations

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	pb "github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/networkconfig/config_expectations_proto"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/networkutils"
	"google.golang.org/protobuf/proto"
)

// IncludesIRDMADriver reports whether the guest OS image includes an in-tree
// irdma driver according to constraints. It defaults to true when no constraint
// matches or when a matching constraint leaves includes_irdma_driver unset.
func IncludesIRDMADriver(constraints []*pb.GuestOsConstraint, image string) (bool, error) {
	imageName := path.Base(strings.TrimSpace(image))
	includes := true
	for _, c := range constraints {
		pattern := c.GetOsPattern()
		if pattern == "" {
			return false, fmt.Errorf("empty os_pattern in guest_os_constraints")
		}
		matched, err := regexp.MatchString(pattern, imageName)
		if err != nil {
			return false, fmt.Errorf("invalid os_pattern %q: %w", pattern, err)
		}
		if matched && c.HasIncludesIrdmaDriver() {
			includes = c.GetIncludesIrdmaDriver()
		}
	}
	return includes, nil
}

// ExpectedNICRegex returns the expected interface name regex for a NIC of
// mdsNICType. When mdsNICType is IRDMA and the guest OS lacks the irdma driver,
// the interface binds only to idpf and uses predictable general-purpose udev
// naming instead of rdma<N>.
func ExpectedNICRegex(mdsNICType string, hasIRDMADriver bool) (*regexp.Regexp, error) {
	if mdsNICType == networkutils.NICTypeIRDMA && !hasIRDMADriver {
		return networkutils.ValidGeneralPurposeNICRe, nil
	}
	return networkutils.ExpectedNICRegex(mdsNICType)
}

// PrepareConfigsForComparison returns normalized clones of want and got ready
// for diffing. When hasIRDMADriver is false, intended IRDMA NICs in want are
// coerced to IDPF and IRQ affinity assertions are cleared on those NICs.
func PrepareConfigsForComparison(want, got *pb.SystemConfig, hasIRDMADriver bool) (*pb.SystemConfig, *pb.SystemConfig) {
	wantClone := proto.Clone(want).(*pb.SystemConfig)
	gotClone := proto.Clone(got).(*pb.SystemConfig)

	for i, wantNic := range wantClone.GetNics() {
		if i >= len(gotClone.GetNics()) {
			break
		}
		gotNic := gotClone.GetNics()[i]

		if wantNic.GetType() == networkutils.NICTypeIRDMA && !hasIRDMADriver {
			wantNic.SetType(networkutils.NICTypeIDPF)
			wantNic.SetRxQueues(nil)
			for _, txQueue := range wantNic.GetTxQueues() {
				txQueue.ClearIrqCpulist()
			}
			gotNic.SetRxQueues(nil)
			for _, txQueue := range gotNic.GetTxQueues() {
				txQueue.ClearIrqCpulist()
			}
		}

		// Only assert optional fields if they're set in the expected config.
		if !wantNic.HasRxRingSize() {
			gotNic.ClearRxRingSize()
		}
		if !wantNic.HasTxRingSize() {
			gotNic.ClearTxRingSize()
		}
		if !wantNic.HasNumaNode() {
			gotNic.ClearNumaNode()
		}
	}

	return wantClone, gotClone
}
