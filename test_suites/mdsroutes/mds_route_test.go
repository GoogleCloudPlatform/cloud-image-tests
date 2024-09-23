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
	ifaces := allIfaces[1:] // Leave out the loopback interface.

	for i, iface := range ifaces {
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
