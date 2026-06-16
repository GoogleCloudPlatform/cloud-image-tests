package networkperf

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestNetworkAddressHelpers(t *testing.T) {
	cases := []struct {
		name       string
		index      int
		wantPrefix string
		wantClient string
		wantServer string
	}{
		{
			name:       "nic_0",
			index:      0,
			wantPrefix: "192.168.0.0/24",
			wantClient: "192.168.0.2",
			wantServer: "192.168.0.3",
		},
		{
			name:       "nic_1",
			index:      1,
			wantPrefix: "192.168.1.0/24",
			wantClient: "192.168.1.2",
			wantServer: "192.168.1.3",
		},
		{
			name:       "nic_2",
			index:      2,
			wantPrefix: "192.168.2.0/24",
			wantClient: "192.168.2.2",
			wantServer: "192.168.2.3",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if networkPrefix(tc.index) != tc.wantPrefix {
				t.Errorf("networkPrefix(%d) = %q, want %q", tc.index, networkPrefix(tc.index), tc.wantPrefix)
			}
			if clientAddress(tc.index) != tc.wantClient {
				t.Errorf("clientAddress(%d) = %q, want %q", tc.index, clientAddress(tc.index), tc.wantClient)
			}
			if serverAddress(tc.index) != tc.wantServer {
				t.Errorf("serverAddress(%d) = %q, want %q", tc.index, serverAddress(tc.index), tc.wantServer)
			}
		})
	}
}

func TestParseNetworkTiers(t *testing.T) {
	tests := []struct {
		name            string
		networkTiersStr string
		want            []networkTier
		wantErr         bool
	}{
		{
			name:            "empty",
			networkTiersStr: "",
			want:            []networkTier{defaultTier},
		},
		{
			name:            "single",
			networkTiersStr: "DEFAULT",
			want:            []networkTier{defaultTier},
		},
		{
			name:            "two_tiers",
			networkTiersStr: "DEFAULT,TIER_1",
			want:            []networkTier{defaultTier, tier1Tier},
		},
		{
			name:            "two_tiers_reversed",
			networkTiersStr: "TIER_1,DEFAULT",
			want:            []networkTier{tier1Tier, defaultTier},
		},
		{
			name:            "two_same_tiers",
			networkTiersStr: "DEFAULT,DEFAULT",
			want:            []networkTier{defaultTier, defaultTier},
		},
		{
			name:            "invalid_tier",
			networkTiersStr: "TIER_1,TIER_2",
			wantErr:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseNetworkTiers(tc.networkTiersStr)
			if tc.wantErr != (err != nil) {
				t.Errorf("parseNetworkTiers(%q) returned an unexpected error: %v", tc.networkTiersStr, err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("parseNetworkTiers(%q) returned an unexpected diff (-want +got):\n%s", tc.networkTiersStr, diff)
			}
		})
	}
}
