// Copyright 2024 Google LLC.
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

package mdsroutes

import (
	"net"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	metadataServerURL = "http://metadata.google.internal/"
)

// Test that only the primary NIC has a route to the MDS.
func TestMDSRoutes(t *testing.T) {
	ctx := utils.Context(t)

	allIfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces() failed: %v", err)
	}
	ifaces := utils.FilterLoopbackTunnelingInterfaces(allIfaces)

	for i, iface := range ifaces {
		// Skip secondary NICs on Windows. Guest agent doesn't manage NICs on
		// windows, so the routes/behavior are more unpredictable.
		if i != 0 && utils.IsWindows() {
			break
		}

		httpClient := &http.Client{
			Timeout: 2 * time.Second,
		}

		// Make a new HTTP request to the metadata server.
		req, err := http.NewRequestWithContext(ctx, "GET", metadataServerURL, nil)
		if err != nil {
			t.Fatalf("http.NewRequestWithContext(ctx, GET, %v, nil) failed: %v", metadataServerURL, err)
		}

		// Obtain its IPv4 address.
		ipAddr, err := utils.ParseInterfaceIPv4(iface)
		if err != nil {
			t.Fatalf("utils.ParseInterfaceIPv4(%v) failed: %v", iface.Name, err)
		}

		// Set up the request to use the NIC.
		req.Header.Add("Metadata-Flavor", "Google")
		dialer := &net.Dialer{
			Timeout:   5 * time.Second,
			LocalAddr: &net.TCPAddr{IP: ipAddr},
		}
		httpClient.Transport = &http.Transport{
			DialContext: dialer.DialContext,
		}

		// Make the request.
		_, err = httpClient.Do(req)
		if err != nil && i == 0 {
			t.Errorf("error connecting to metadata server on primary nic %s: %v", iface.Name, err)
		} else if err == nil && i != 0 {
			t.Errorf("unexpected success connecting to metadata server on nic %s", iface.Name)
		}
	}
}

func TestDNS(t *testing.T) {
	if _, err := exec.LookPath("dig"); err != nil {
		t.Skipf("error finding dig: %v", err)
	}
	ctx := utils.Context(t)

	// TCP test.
	cmd := exec.CommandContext(ctx, "dig", "+tcp", "@169.254.169.254", "www.google.com")
	if res, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("error running dig tcp: %v: %s", err, string(res))
	} else {
		t.Logf("dig tcp output: %s", string(res))
	}

	// UDP test.
	cmd = exec.CommandContext(ctx, "dig", "+notcp", "@169.254.169.254", "www.google.com")
	if res, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("error running dig udp: %v: %s", err, string(res))
	} else {
		t.Logf("dig udp output: %s", string(res))
	}
}

func TestWindowsDNS(t *testing.T) {
	ctx := utils.Context(t)

	// Run nslookup
	cmd := exec.CommandContext(ctx, "nslookup", "www.google.com", "169.254.169.254")
	if res, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("error running nslookup: %v: %s", err, string(res))
	} else {
		t.Logf("nslookup output: %s", string(res))
	}
}
