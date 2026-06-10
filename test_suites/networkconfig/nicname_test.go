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
	"fmt"
	"regexp"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

var (
	validGeneralPurposeRegex = regexp.MustCompile(`^(eth|ens)\d+$`)
	validIRDMARegex          = regexp.MustCompile(`^rdma\d+$`)
	validMRDMARegex          = regexp.MustCompile(`^gpu\d+-rdma\d+$`)
)

func expectedNICRegex(nicType string) (*regexp.Regexp, error) {
	switch nicType {
	case nicTypeVIRTIONET:
		return validGeneralPurposeRegex, nil
	case nicTypeGVNIC:
		return validGeneralPurposeRegex, nil
	case nicTypeIDPF:
		return validGeneralPurposeRegex, nil
	case nicTypeIRDMA:
		return validIRDMARegex, nil
	case nicTypeMRDMA:
		return validMRDMARegex, nil
	default:
		return nil, fmt.Errorf("unknown/unsupported NIC type: %q", nicType)
	}
}

func TestNICNames(t *testing.T) {
	ctx := utils.Context(t)
	mdsIfaces, err := listMDSIfaces(ctx)
	if err != nil {
		t.Fatalf("listMDSIfaces(ctx) = err %v want nil", err)
	}
	if len(mdsIfaces) == 0 {
		t.Fatalf("no network interfaces found in metadata")
	}

	seenIfaceNames := make(map[string]bool)
	for _, mdsIface := range mdsIfaces {
		systemIface, err := utils.GetInterfaceByMAC(mdsIface.MAC)
		if err != nil {
			t.Errorf("GetInterfaceByMAC(%q) = err %v want nil", mdsIface.MAC, err)
			continue
		}

		if seenIfaceNames[systemIface.Name] {
			t.Errorf("duplicate NIC name %q found", systemIface.Name)
		}
		seenIfaceNames[systemIface.Name] = true

		t.Run(systemIface.Name, func(t *testing.T) {
			regex, err := expectedNICRegex(mdsIface.NICType)
			if err != nil {
				t.Errorf("expectedNICRegex(%q) = err %v want nil", mdsIface.NICType, err)
				return
			}

			if regex.MatchString(systemIface.Name) {
				t.Logf("NIC name %q matches naming expectations for NIC type %q", systemIface.Name, mdsIface.NICType)
			} else {
				t.Errorf("NIC name %q does not match expected regex %q for NIC type %q", systemIface.Name, regex.String(), mdsIface.NICType)
			}
		})
	}
}
