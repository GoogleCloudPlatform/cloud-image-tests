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

package expectations

import (
	"testing"

	pb "github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/networkconfig/config_expectations_proto"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/networkutils"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestIncludesIRDMADriver(t *testing.T) {
	defaultConstraints := []*pb.GuestOsConstraint{
		pb.GuestOsConstraint_builder{
			OsPattern:           proto.String("debian-13"),
			IncludesIrdmaDriver: proto.Bool(false),
		}.Build(),
	}

	tests := []struct {
		name        string
		constraints []*pb.GuestOsConstraint
		image       string
		want        bool
	}{
		{
			name:        "empty_constraints_defaults_true",
			constraints: nil,
			image:       "projects/debian-cloud/global/images/debian-13-trixie-v20261015",
			want:        true,
		},
		{
			name:        "debian_13_public_image_overridden_false",
			constraints: defaultConstraints,
			image:       "projects/debian-cloud/global/images/debian-13-trixie-v20261015",
			want:        false,
		},
		{
			name:        "debian_13_baked_staging_image_overridden_false",
			constraints: defaultConstraints,
			image:       "projects/guest-net-configurer-staging/global/images/debian-13-a80640b9",
			want:        false,
		},
		{
			name:        "ubuntu_defaults_true",
			constraints: defaultConstraints,
			image:       "projects/ubuntu-os-cloud/global/images/ubuntu-2404-noble-amd64-v20261020",
			want:        true,
		},
		{
			name:        "cos_stable_defaults_true",
			constraints: defaultConstraints,
			image:       "projects/cos-cloud/global/images/cos-129-19216-104-45",
			want:        true,
		},
		{
			name:        "project_name_not_matched",
			constraints: defaultConstraints,
			image:       "projects/my-debian-13-project/global/images/ubuntu-2404-noble-amd64",
			want:        true,
		},
		{
			name: "unset_field_defaults_true",
			constraints: []*pb.GuestOsConstraint{
				pb.GuestOsConstraint_builder{
					OsPattern: proto.String("debian-13"),
				}.Build(),
			},
			image: "projects/debian-cloud/global/images/debian-13-trixie-v20261015",
			want:  true,
		},
		{
			name: "later_matching_constraint_overrides_earlier",
			constraints: []*pb.GuestOsConstraint{
				pb.GuestOsConstraint_builder{
					OsPattern:           proto.String("debian-"),
					IncludesIrdmaDriver: proto.Bool(false),
				}.Build(),
				pb.GuestOsConstraint_builder{
					OsPattern:           proto.String("debian-14"),
					IncludesIrdmaDriver: proto.Bool(true),
				}.Build(),
			},
			image: "projects/debian-cloud/global/images/debian-14-forky-v20270101",
			want:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := IncludesIRDMADriver(tc.constraints, tc.image)
			if err != nil {
				t.Fatalf("IncludesIRDMADriver(..., %q) unexpected error: %v", tc.image, err)
			}
			if got != tc.want {
				t.Errorf("IncludesIRDMADriver(..., %q) = %t, want %t", tc.image, got, tc.want)
			}
		})
	}
}

func TestIncludesIRDMADriver_InvalidConstraints(t *testing.T) {
	tests := []struct {
		name        string
		constraints []*pb.GuestOsConstraint
	}{
		{
			name: "empty_pattern",
			constraints: []*pb.GuestOsConstraint{
				pb.GuestOsConstraint_builder{
					OsPattern:           proto.String(""),
					IncludesIrdmaDriver: proto.Bool(false),
				}.Build(),
			},
		},
		{
			name: "invalid_regex",
			constraints: []*pb.GuestOsConstraint{
				pb.GuestOsConstraint_builder{
					OsPattern:           proto.String("[unclosed"),
					IncludesIrdmaDriver: proto.Bool(false),
				}.Build(),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := IncludesIRDMADriver(tc.constraints, "debian-13"); err == nil {
				t.Errorf("IncludesIRDMADriver(%v, \"debian-13\") = nil error, want non-nil", tc.constraints)
			}
		})
	}
}

func TestExpectedNICRegex(t *testing.T) {
	tests := []struct {
		name           string
		mdsNICType     string
		hasIRDMADriver bool
		sampleNICName  string
		wantMatch      bool
	}{
		{
			name:           "irdma_with_driver_matches_rdma",
			mdsNICType:     networkutils.NICTypeIRDMA,
			hasIRDMADriver: true,
			sampleNICName:  "rdma0",
			wantMatch:      true,
		},
		{
			name:           "irdma_with_driver_rejects_ens",
			mdsNICType:     networkutils.NICTypeIRDMA,
			hasIRDMADriver: true,
			sampleNICName:  "ens7",
			wantMatch:      false,
		},
		{
			name:           "irdma_without_driver_matches_ens",
			mdsNICType:     networkutils.NICTypeIRDMA,
			hasIRDMADriver: false,
			sampleNICName:  "ens7",
			wantMatch:      true,
		},
		{
			name:           "irdma_without_driver_matches_enp",
			mdsNICType:     networkutils.NICTypeIRDMA,
			hasIRDMADriver: false,
			sampleNICName:  "enp128s4",
			wantMatch:      true,
		},
		{
			name:           "irdma_without_driver_matches_vf",
			mdsNICType:     networkutils.NICTypeIRDMA,
			hasIRDMADriver: false,
			sampleNICName:  "enp22s0f0v0",
			wantMatch:      true,
		},
		{
			name:           "irdma_without_driver_rejects_rdma",
			mdsNICType:     networkutils.NICTypeIRDMA,
			hasIRDMADriver: false,
			sampleNICName:  "rdma0",
			wantMatch:      false,
		},
		{
			name:           "irdma_without_driver_rejects_udev_tmp",
			mdsNICType:     networkutils.NICTypeIRDMA,
			hasIRDMADriver: false,
			sampleNICName:  "eth0tmp",
			wantMatch:      false,
		},
		{
			name:           "mrdma_unaffected_without_irdma_driver",
			mdsNICType:     networkutils.NICTypeMRDMA,
			hasIRDMADriver: false,
			sampleNICName:  "gpu0rdma0",
			wantMatch:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			re, err := ExpectedNICRegex(tc.mdsNICType, tc.hasIRDMADriver)
			if err != nil {
				t.Fatalf("ExpectedNICRegex(%q, %t) unexpected error: %v", tc.mdsNICType, tc.hasIRDMADriver, err)
			}
			if got := re.MatchString(tc.sampleNICName); got != tc.wantMatch {
				t.Errorf("ExpectedNICRegex(%q, %t).MatchString(%q) = %t, want %t", tc.mdsNICType, tc.hasIRDMADriver, tc.sampleNICName, got, tc.wantMatch)
			}
		})
	}
}

func TestPrepareConfigsForComparison(t *testing.T) {
	want := pb.SystemConfig_builder{
		Nics: []*pb.NicExpectation{
			pb.NicExpectation_builder{
				Type: proto.String(networkutils.NICTypeIDPF),
				TxQueues: []*pb.TxQueue{
					pb.TxQueue_builder{Index: proto.Int32(0), IrqCpulist: proto.String("1"), XpsCpulist: proto.String("0-11")}.Build(),
				},
				RxQueues: []*pb.RxQueue{
					pb.RxQueue_builder{Index: proto.Int32(0), IrqCpulist: proto.String("1")}.Build(),
				},
			}.Build(),
			pb.NicExpectation_builder{
				Type:       proto.String(networkutils.NICTypeIRDMA),
				RxRingSize: proto.Int32(4096),
				TxRingSize: proto.Int32(4096),
				TxQueues: []*pb.TxQueue{
					pb.TxQueue_builder{Index: proto.Int32(0), XpsCpulist: proto.String("0-23")}.Build(),
				},
			}.Build(),
		},
	}.Build()

	gotFallbackIDPF := pb.SystemConfig_builder{
		Nics: []*pb.NicExpectation{
			pb.NicExpectation_builder{
				Type: proto.String(networkutils.NICTypeIDPF),
				TxQueues: []*pb.TxQueue{
					pb.TxQueue_builder{Index: proto.Int32(0), IrqCpulist: proto.String("1"), XpsCpulist: proto.String("0-11")}.Build(),
				},
				RxQueues: []*pb.RxQueue{
					pb.RxQueue_builder{Index: proto.Int32(0), IrqCpulist: proto.String("1")}.Build(),
				},
				RxRingSize: proto.Int32(4096),
				TxRingSize: proto.Int32(4096),
				NumaNode:   proto.Int32(0),
			}.Build(),
			pb.NicExpectation_builder{
				Type:       proto.String(networkutils.NICTypeIDPF),
				RxRingSize: proto.Int32(4096),
				TxRingSize: proto.Int32(4096),
				NumaNode:   proto.Int32(0),
				TxQueues: []*pb.TxQueue{
					pb.TxQueue_builder{Index: proto.Int32(0), IrqCpulist: proto.String("17"), XpsCpulist: proto.String("0-23")}.Build(),
				},
				RxQueues: []*pb.RxQueue{
					pb.RxQueue_builder{Index: proto.Int32(0), IrqCpulist: proto.String("17")}.Build(),
				},
			}.Build(),
		},
	}.Build()

	t.Run("coerces_irdma_to_idpf_when_driver_absent", func(t *testing.T) {
		wantNorm, gotNorm := PrepareConfigsForComparison(want, gotFallbackIDPF, false)
		if diff := cmp.Diff(wantNorm, gotNorm, protocmp.Transform()); diff != "" {
			t.Errorf("PrepareConfigsForComparison(..., false) mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("preserves_irdma_expectation_when_driver_present", func(t *testing.T) {
		wantNorm, gotNorm := PrepareConfigsForComparison(want, gotFallbackIDPF, true)
		if diff := cmp.Diff(wantNorm, gotNorm, protocmp.Transform()); diff == "" {
			t.Errorf("PrepareConfigsForComparison(..., true) unexpectedly matched fallback IDPF NIC")
		}
	})

	t.Run("still_checks_xps_and_ring_sizes_on_fallback_idpf", func(t *testing.T) {
		badXPS := proto.Clone(gotFallbackIDPF).(*pb.SystemConfig)
		badXPS.GetNics()[1].GetTxQueues()[0].SetXpsCpulist("24-47")
		wantNorm, gotNorm := PrepareConfigsForComparison(want, badXPS, false)
		if diff := cmp.Diff(wantNorm, gotNorm, protocmp.Transform()); diff == "" {
			t.Errorf("PrepareConfigsForComparison(..., false) failed to catch XPS mismatch on fallback IDPF NIC")
		}
	})
}
