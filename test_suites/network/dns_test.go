// Copyright 2025 Google LLC.
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

package network

import (
	"context"
	"fmt"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"google.golang.org/api/dns/v1"
)

func setupLocalCloudDNS(ctx context.Context, t *testing.T, ip string) string {
	t.Helper()

	cloudDNS, err := dns.NewService(ctx)
	if err != nil {
		t.Fatalf("Failed to create Cloud DNS service: %v", err)
	}

	network, err := utils.GetMetadata(ctx, "instance", "network-interfaces", "0", "network")
	if err != nil {
		t.Fatalf("Failed to get network: %v", err)
	}

	networkName := network[strings.LastIndex(network, "/")+1:]

	project, err := utils.GetMetadata(ctx, "project", "project-id")
	if err != nil {
		t.Fatalf("Failed to get project ID: %v", err)
	}

	managedZone, err := cloudDNS.ManagedZones.Create(project, &dns.ManagedZone{
		Name:        "test-local-zone",
		DnsName:     "testlocalzone.com.",
		Visibility:  "private",
		Description: "Test local Cloud DNS zone",
		PrivateVisibilityConfig: &dns.ManagedZonePrivateVisibilityConfig{
			Networks: []*dns.ManagedZonePrivateVisibilityConfigNetwork{
				{
					Kind:       "dns#managedZonePrivateVisibilityConfigNetwork",
					NetworkUrl: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networks/%s", project, networkName),
				},
			},
		},
	}).Context(ctx).Do()
	if err != nil {
		t.Fatalf("Failed to create local Cloud DNS zone: %v, \nResponse: %+v", err, managedZone)
	}

	rrset, err := cloudDNS.ResourceRecordSets.Create(project, managedZone.Name, &dns.ResourceRecordSet{
		Name:    "demo.testlocalzone.com.",
		Type:    "A",
		Rrdatas: []string{ip},
	}).Context(ctx).Do()
	if err != nil {
		t.Fatalf("Failed to create local Cloud DNS A record: %v, \nResponse: %+v", err, rrset)
	}

	t.Cleanup(func() {
		if resp, err := cloudDNS.ResourceRecordSets.Delete(project, managedZone.Name, rrset.Name, "A").Context(ctx).Do(); err != nil {
			t.Fatalf("Failed to delete local Cloud DNS A record: %v, \nResponse: %+v", err, resp)
		}
		if err := cloudDNS.ManagedZones.Delete(project, managedZone.Name).Context(ctx).Do(); err != nil {
			t.Fatalf("Failed to delete local Cloud DNS zone: %v", err)
		}
	})

	return rrset.Name
}

func TestLocalCloudDNS(t *testing.T) {
	ctx := context.Background()

	name := setupLocalCloudDNS(ctx, t, vm1Config.ip)

	// Add some delay for DNS record to propagate.
	time.Sleep(5 * time.Second)

	ips, err := net.LookupIP(name)
	if err != nil {
		t.Fatalf("Failed to lookup IP for local Cloud DNS name %q: %v", name, err)
	}

	targetIP := net.ParseIP(vm1Config.ip)

	found := slices.ContainsFunc(ips, func(ip net.IP) bool {
		return ip.Equal(targetIP)
	})

	if !found {
		readableIPs := make([]string, len(ips))
		for i, ip := range ips {
			readableIPs[i] = ip.String()
		}
		t.Fatalf("Local Cloud DNS IP %v does not contain VM IP %v", readableIPs, vm1Config.ip)
	}

	if utils.IsWindows() {
		return
	}

	searchPath, contents := readSearchPath(t)
	if strings.Contains(searchPath, " local ") {
		t.Fatalf("/etc/resolv.conf contains local in search path: [%q]\nContents:\n %s", searchPath, contents)
	}
}

func readSearchPath(t *testing.T) (string, string) {
	t.Helper()
	contents, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		t.Fatalf("Failed to read /etc/resolv.conf: %v", err)
	}

	lines := strings.Split(string(contents), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "search") {
			return line, string(contents)
		}
	}
	return "", string(contents)
}
