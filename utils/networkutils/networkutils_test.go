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

package networkutils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/GoogleCloudPlatform/compute-daisy"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/compute/v1"
)

// makeRange returns a slice of integers from low to high, inclusive.
func makeRange(low int, high int) []int {
	var result []int
	for i := low; i <= high; i++ {
		result = append(result, i)
	}
	return result
}

func TestParseCpusetMask(t *testing.T) {
	tests := []struct {
		name    string
		mask    string
		want    *Cpuset
		wantErr bool
	}{
		{
			name:    "invalid hex",
			mask:    "invalid",
			want:    nil,
			wantErr: true,
		},
		{
			name: "empty mask",
			mask: "",
			want: &Cpuset{},
		},
		{
			name: "single cpu 0",
			mask: "1",
			want: &Cpuset{cpus: []int{0}},
		},
		{
			name: "single cpu 1",
			mask: "2",
			want: &Cpuset{cpus: []int{1}},
		},
		{
			name: "single cpu 3",
			mask: "8",
			want: &Cpuset{cpus: []int{3}},
		},
		{
			name: "multiple cpus 0 and 1",
			mask: "3",
			want: &Cpuset{cpus: []int{0, 1}},
		},
		{
			name: "multiple cpus 0, 1, 2, 3",
			mask: "f",
			want: &Cpuset{cpus: []int{0, 1, 2, 3}},
		},
		{
			name: "comma separated mask low bits",
			mask: "00000000,00000003",
			want: &Cpuset{cpus: []int{0, 1}},
		},
		{
			name: "comma separated mask high bits",
			mask: "00000003,00000000",
			want: &Cpuset{cpus: []int{32, 33}},
		},
		{
			name: "spaces trimmed",
			mask: "  f  ",
			want: &Cpuset{cpus: []int{0, 1, 2, 3}},
		},
		{
			name: "real config, a3 half cpus",
			mask: "0000,00000000,0fffffff,ffffff00,00000000,000fffff,ffffffff",
			want: &Cpuset{cpus: append(makeRange(0, 51), makeRange(104, 155)...)},
		},
		{
			name: "real config, a4 high cpus",
			mask: "ffffe000,00000000,00000000,00000000,00000000,00000000,00000000",
			want: &Cpuset{cpus: makeRange(205, 223)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCpusetMask(tc.mask)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseHexMask(%q) returned error %v, wantErr %t", tc.mask, err, tc.wantErr)
				return
			}
			if diff := cmp.Diff(got, tc.want, cmp.AllowUnexported(Cpuset{})); diff != "" {
				t.Errorf("parseHexMask(%q) doesn't match expectations: diff (-got +want):\n%s", tc.mask, diff)
			}
		})
	}
}

func TestCpusetString(t *testing.T) {
	tests := []struct {
		name   string
		cpuset *Cpuset
		want   string
	}{
		{
			name:   "empty",
			cpuset: &Cpuset{},
			want:   "",
		},
		{
			name:   "single",
			cpuset: &Cpuset{cpus: []int{1}},
			want:   "1",
		},
		{
			name:   "contiguous range",
			cpuset: &Cpuset{cpus: []int{0, 1, 2}},
			want:   "0-2",
		},
		{
			name:   "disjoint ranges",
			cpuset: &Cpuset{cpus: []int{0, 1, 4, 5}},
			want:   "0-1,4-5",
		},
		{
			name:   "mixed contiguous and single",
			cpuset: &Cpuset{cpus: []int{0, 1, 3, 5, 6}},
			want:   "0-1,3,5-6",
		},
		{
			name:   "unsorted input",
			cpuset: &Cpuset{cpus: []int{5, 0, 6, 3, 1}},
			want:   "0-1,3,5-6",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cpuset.ListString()
			if got != tc.want {
				t.Errorf("cpuListString(%v) = %q, want %q", tc.cpuset, got, tc.want)
			}
		})
	}
}

func TestEthtoolDriverRegex(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{
			name:   "standard virtio_net",
			input:  "driver: virtio_net\nversion: 1.0.0\nfirmware-version: \nexpansion-rom-version: \nbus-info: 0000:00:04.0\nsupports-statistics: yes\nsupports-test: no\nsupports-eeprom-access: no\nsupports-register-dump: no\nsupports-priv-flags: no\n",
			want:   "virtio_net",
			wantOK: true,
		},
		{
			name:   "gve driver",
			input:  "driver: gve\nversion: 1.0.0\n",
			want:   "gve",
			wantOK: true,
		},
		{
			name:   "no driver line",
			input:  "version: 1.0.0\n",
			wantOK: false,
		},
		{
			name:   "driver line not at start",
			input:  "something else\ndriver: virtio_net\n",
			want:   "virtio_net",
			wantOK: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matches := EthtoolDriverRe.FindStringSubmatch(tc.input)
			if !tc.wantOK {
				if len(matches) >= 2 {
					t.Errorf("expected no match, got %v", matches)
				}
				return
			}
			if len(matches) < 2 {
				t.Fatalf("expected match, got none")
			}
			got := matches[1]
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestListMDSIfaces(t *testing.T) {
	ctx := context.Background()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/computeMetadata/v1/instance/network-interfaces" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("recursive") != "true" {
			http.Error(w, "expected recursive query", http.StatusBadRequest)
			return
		}
		if r.Header.Get("Metadata-Flavor") != "Google" {
			http.Error(w, "missing metadata flavor header", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"mac": "00:11:22:33:44:55", "nicType": "GVNIC"},
			{"mac": "66:77:88:99:aa:bb", "nicType": "VIRTIO_NET"}
		]`))
	}))
	defer ts.Close()

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("failed to parse test server URL: %v", err)
	}

	t.Setenv("GCE_METADATA_HOST", u.Host)

	got, err := ListMDSIfaces(ctx)
	if err != nil {
		t.Fatalf("ListMDSIfaces(ctx) failed: %v", err)
	}

	want := []NetworkInterface{
		{MAC: "00:11:22:33:44:55", NICType: "GVNIC"},
		{MAC: "66:77:88:99:aa:bb", NICType: "VIRTIO_NET"},
	}

	if diff := cmp.Diff(got, want); diff != "" {
		t.Errorf("ListMDSIfaces(ctx) = doesn't match expectations: diff (-got +want):\n%s", diff)
	}
}

func TestExpandNICTypes(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{
			name:    "empty defaults to gvnic",
			input:   "",
			want:    []string{"GVNIC"},
			wantErr: false,
		},
		{
			name:    "single",
			input:   "a:1",
			want:    []string{"a"},
			wantErr: false,
		},
		{
			name:    "double",
			input:   "a:2",
			want:    []string{"a", "a"},
			wantErr: false,
		},
		{
			name:    "two kinds",
			input:   "a:1,b:2",
			want:    []string{"a", "b", "b"},
			wantErr: false,
		},
		{
			name:    "no number",
			input:   "a",
			wantErr: true,
		},
		{
			name:    "trims spaces",
			input:   " a:1 , b:2 ",
			want:    []string{"a", "b", "b"},
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExpandNICTypes(tc.input)
			if (err != nil) != tc.wantErr {
				if tc.wantErr {
					t.Errorf("ExpandNICTypes(%q) = (%v, %v), want (%v, %s)", tc.input, got, err, tc.want, "non-nil")
				} else {
					t.Errorf("ExpandNICTypes(%q) = (%v, %v), want (%v, %s)", tc.input, got, err, tc.want, "nil")
				}
			}

			if !cmp.Equal(got, tc.want) {
				t.Errorf("ExpandNICTypes(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func diffDaisy(a any, b any) string {
	return cmp.Diff(a, b, cmpopts.IgnoreUnexported(daisy.Resource{}))
}

func TestDaisyNetworkForNIC(t *testing.T) {
	cases := []struct {
		name    string
		nicType string
		index   int
		project string
		zone    string
		isMetal bool
		want    *daisy.Network
	}{
		{
			name:    "VIRTIO NIC",
			nicType: NICTypeVIRTIONET,
			index:   0,
			want: &daisy.Network{
				Network: compute.Network{
					Name: "network-0",
					Mtu:  8896,
				},
				AutoCreateSubnetworks: new(bool),
			},
		},
		{
			name:    "GVNIC NIC",
			nicType: NICTypeGVNIC,
			index:   0,
			want: &daisy.Network{
				Network: compute.Network{
					Name: "network-0",
					Mtu:  8896,
				},
				AutoCreateSubnetworks: new(bool),
			},
		},
		{
			name:    "IDPF NIC",
			nicType: NICTypeIDPF,
			index:   0,
			want: &daisy.Network{
				Network: compute.Network{
					Name: "network-0",
					Mtu:  8896,
				},
				AutoCreateSubnetworks: new(bool),
			},
		},
		{
			name:    "IRDMA NIC",
			nicType: NICTypeIRDMA,
			index:   0,
			project: "test-project",
			zone:    "us-central1-a",
			want: &daisy.Network{
				Network: compute.Network{
					Name:           "irdma-network-0",
					Mtu:            8896,
					NetworkProfile: "https://www.googleapis.com/compute/v1/projects/test-project/global/networkProfiles/us-central1-a-vpc-falcon",
				},
				AutoCreateSubnetworks: new(bool),
			},
		},
		{
			name:    "MRDMA virtual NIC",
			nicType: NICTypeMRDMA,
			index:   0,
			project: "test-project",
			zone:    "europe-central2-a",
			want: &daisy.Network{
				Network: compute.Network{
					Name:           "mrdma-network-0",
					Mtu:            8896,
					NetworkProfile: "https://www.googleapis.com/compute/v1/projects/test-project/global/networkProfiles/europe-central2-a-vpc-roce",
				},
				AutoCreateSubnetworks: new(bool),
			},
		},
		{
			name:    "MRDMA metal NIC",
			nicType: NICTypeMRDMA,
			index:   0,
			project: "test-project",
			zone:    "asia-southeast2-b",
			isMetal: true,
			want: &daisy.Network{
				Network: compute.Network{
					Name:           "mrdma-network-0",
					Mtu:            8896,
					NetworkProfile: "https://www.googleapis.com/compute/v1/projects/test-project/global/networkProfiles/asia-southeast2-b-vpc-roce-metal",
				},
				AutoCreateSubnetworks: new(bool),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := daisyNetworkForNIC(tc.nicType, tc.index, tc.project, tc.zone, tc.isMetal)
			if err != nil {
				t.Fatalf("daisyNetworkForNIC(%q, %d, %q, %q, %t) failed: %v", tc.nicType, tc.index, tc.project, tc.zone, tc.isMetal, err)
			}
			if diff := diffDaisy(got, tc.want); diff != "" {
				t.Errorf("daisyNetworkForNIC(%q, %d, %q, %q, %t) = doesn't match expectations: diff (-got +want):\n%s", tc.nicType, tc.index, tc.project, tc.zone, tc.isMetal, diff)
			}
		})
	}
}

func TestDaisySubnet(t *testing.T) {
	cases := []struct {
		name    string
		index   int
		zone    string
		want    *daisy.Subnetwork
		wantErr bool
	}{
		{
			name:  "index 0",
			index: 0,
			zone:  "us-central1-a",
			want: &daisy.Subnetwork{
				Subnetwork: compute.Subnetwork{
					Name:        "subnet-0",
					IpCidrRange: "10.0.0.0/24",
					Region:      "us-central1",
				},
			},
		},
		{
			name:  "index 1",
			index: 1,
			zone:  "europe-west1-b",
			want: &daisy.Subnetwork{
				Subnetwork: compute.Subnetwork{
					Name:        "subnet-1",
					IpCidrRange: "10.0.1.0/24",
					Region:      "europe-west1",
				},
			},
		},
		{
			name:    "index too low",
			index:   -1,
			wantErr: true,
		},
		{
			name:    "index too high",
			index:   256,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := daisySubnet(tc.index, tc.zone)
			if (err != nil) != tc.wantErr {
				t.Errorf("daisySubnet(%d, %q) returned error %v, wantErr %t", tc.index, tc.zone, err, tc.wantErr)
				return
			}
			if diff := diffDaisy(got, tc.want); diff != "" {
				t.Errorf("daisySubnet(%d, %q) = doesn't match expectations: diff (-got +want):\n%s", tc.index, tc.zone, diff)
			}
		})
	}
}

func TestEthtoolLRegex(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    *EthtoolQueueCounts
		wantErr bool
	}{
		{
			name: "gve 16 queues",
			input: `Channel parameters for enp0s19:
Pre-set maximums:
RX:		16
TX:		16
Other:		n/a
Combined:	n/a
Current hardware settings:
RX:		8
TX:		8
Other:		n/a
Combined:	n/a`,
			want: &EthtoolQueueCounts{
				MaxRXQueues:           16,
				MaxTXQueues:           16,
				MaxOtherQueues:        -1,
				MaxCombinedQueues:     -1,
				CurrentRXQueues:       8,
				CurrentTXQueues:       8,
				CurrentOtherQueues:    -1,
				CurrentCombinedQueues: -1,
			},
		},
		{
			name: "idpf or virtio 16 queues",
			input: `Channel parameters for enp0s19:
Pre-set maximums:
RX:		n/a
TX:		n/a
Other:		n/a
Combined:	64
Current hardware settings:
RX:		n/a
TX:		n/a
Other:		n/a
Combined:	16`,
			want: &EthtoolQueueCounts{
				MaxRXQueues:           -1,
				MaxTXQueues:           -1,
				MaxOtherQueues:        -1,
				MaxCombinedQueues:     64,
				CurrentRXQueues:       -1,
				CurrentTXQueues:       -1,
				CurrentOtherQueues:    -1,
				CurrentCombinedQueues: 16,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseEthtoolLOutput(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("ParseEthtoolLOutput(%q) returned error %v, wantErr %t", tc.input, err, tc.wantErr)
				return
			}
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("ParseEthtoolLOutput(%q) = doesn't match expectations: diff (-got +want):\n%s", tc.input, diff)
			}
		})
	}
}
