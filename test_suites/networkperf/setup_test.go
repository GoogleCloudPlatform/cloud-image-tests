package networkperf

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests"
	daisycompute "github.com/GoogleCloudPlatform/compute-daisy/compute"
	"github.com/google/go-cmp/cmp"
)

// newTestWorkflow returns a TestWorkflow backed by a fake compute API that
// serves just enough for imagetest.NewTestWorkflow to succeed.
func newTestWorkflow(t *testing.T) *imagetest.TestWorkflow {
	t.Helper()
	srv, client, err := daisycompute.NewTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.String() {
		case "/projects/fake-project?alt=json&prettyPrint=false":
			fmt.Fprint(w, `{"Name":"fake-project"}`)
		case "/projects/fake-project/zones/us-central1-a?alt=json&prettyPrint=false":
			fmt.Fprint(w, `{"Name":"us-central1-a"}`)
		case "/projects/fake-project/zones/us-central1-a/machineTypes/n1-standard-1?alt=json&prettyPrint=false":
			fmt.Fprint(w, `{"Name":"n1-standard-1"}`)
		case "/projects/fake-cloud/global/images/fakeos?alt=json&prettyPrint=false":
			fmt.Fprint(w, `{"Name":"fakeos", "Architecture":"X86_64"}`)
		default:
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, "URL and Method not recognized:", r.Method, r.URL)
		}
	}))
	if err != nil {
		t.Fatalf("failed to create fake compute client: %v", err)
	}
	t.Cleanup(srv.Close)
	twf, err := imagetest.NewTestWorkflow(&imagetest.TestWorkflowOpts{
		Client:   client,
		Name:     "networkperf",
		Image:    "projects/fake-cloud/global/images/fakeos",
		Timeout:  "20m",
		Project:  "fake-project",
		Zone:     "us-central1-a",
		X86Shape: "n1-standard-1",
	}, nil)
	if err != nil {
		t.Fatalf("failed to create test workflow: %v", err)
	}
	return twf
}

func TestCreateMachine(t *testing.T) {
	tests := []struct {
		name          string
		machinePrefix string
	}{
		{name: "client", machinePrefix: "client"},
		{name: "server", machinePrefix: "server"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			twf := newTestWorkflow(t)
			networks, err := createNetworks(twf, &createNetworkOptions{namePrefix: "net", region: "us-central1", nicTypes: []string{"GVNIC"}})
			if err != nil {
				t.Fatalf("createNetworks failed: %v", err)
			}
			vm, err := createMachine(twf, "TestNetworkPerformance", tc.machinePrefix, networks, "n1-standard-1", "us-central1-a")
			if err != nil {
				t.Fatalf("createMachine failed: %v", err)
			}
			if vm == nil {
				t.Fatal("createMachine returned a nil vm")
			}
		})
	}
}

func TestCreateMachineUnknownPrefix(t *testing.T) {
	twf := newTestWorkflow(t)
	networks, err := createNetworks(twf, &createNetworkOptions{namePrefix: "net", region: "us-central1", nicTypes: []string{"GVNIC"}})
	if err != nil {
		t.Fatalf("createNetworks failed: %v", err)
	}
	if _, err := createMachine(twf, "TestNetworkPerformance", "other", networks, "n1-standard-1", "us-central1-a"); err == nil {
		t.Error("createMachine succeeded with an unknown machine prefix, want error")
	}
}

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
