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
	"reflect"
	"testing"
)

// makeRange returns a slice of integers from low to high, inclusive.
func makeRange(low int, high int) []int {
	var result []int
	for i := low; i <= high; i++ {
		result = append(result, i)
	}
	return result
}

func TestParseHexMask(t *testing.T) {
	tests := []struct {
		name    string
		mask    string
		want    []int
		wantErr bool
	}{
		{
			name:    "empty mask",
			mask:    "",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid hex",
			mask:    "invalid",
			want:    nil,
			wantErr: true,
		},
		{
			name: "single cpu 0",
			mask: "1",
			want: []int{0},
		},
		{
			name: "single cpu 1",
			mask: "2",
			want: []int{1},
		},
		{
			name: "single cpu 3",
			mask: "8",
			want: []int{3},
		},
		{
			name: "multiple cpus 0 and 1",
			mask: "3",
			want: []int{0, 1},
		},
		{
			name: "multiple cpus 0, 1, 2, 3",
			mask: "f",
			want: []int{0, 1, 2, 3},
		},
		{
			name: "comma separated mask low bits",
			mask: "00000000,00000003",
			want: []int{0, 1},
		},
		{
			name: "comma separated mask high bits",
			mask: "00000003,00000000",
			want: []int{32, 33},
		},
		{
			name: "spaces trimmed",
			mask: "  f  ",
			want: []int{0, 1, 2, 3},
		},
		{
			name: "real config, a3 half cpus",
			mask: "0000,00000000,0fffffff,ffffff00,00000000,000fffff,ffffffff",
			want: append(makeRange(0, 51), makeRange(104, 155)...),
		},
		{
			name: "real config, a4 high cpus",
			mask: "ffffe000,00000000,00000000,00000000,00000000,00000000,00000000",
			want: makeRange(205, 223),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHexMask(tc.mask)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseHexMask(%q) returned error %v, wantErr %t", tc.mask, err, tc.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseHexMask(%q) = %v, want %v", tc.mask, got, tc.want)
			}
		})
	}
}

func TestCpusetString(t *testing.T) {
	tests := []struct {
		name string
		cpus []int
		want string
	}{
		{
			name: "empty",
			cpus: nil,
			want: "",
		},
		{
			name: "single",
			cpus: []int{1},
			want: "1",
		},
		{
			name: "contiguous range",
			cpus: []int{0, 1, 2},
			want: "0-2",
		},
		{
			name: "disjoint ranges",
			cpus: []int{0, 1, 4, 5},
			want: "0-1,4-5",
		},
		{
			name: "mixed contiguous and single",
			cpus: []int{0, 1, 3, 5, 6},
			want: "0-1,3,5-6",
		},
		{
			name: "unsorted input",
			cpus: []int{5, 0, 6, 3, 1},
			want: "0-1,3,5-6",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cpuListString(tc.cpus)
			if got != tc.want {
				t.Errorf("cpuListString(%v) = %q, want %q", tc.cpus, got, tc.want)
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
			matches := ethtoolDriverRe.FindStringSubmatch(tc.input)
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
