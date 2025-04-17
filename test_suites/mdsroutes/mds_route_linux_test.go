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

//go:build linux

package mdsroutes

import (
	"context"
	"net"
	"net/http"
	"syscall"
	"testing"
)

func metadataRequest(ctx context.Context, t *testing.T, iface net.Interface) error {
	t.Helper()

	dialer := &net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			var controlErr error
			err := c.Control(func(fd uintptr) {
				controlErr = syscall.SetsockoptString(
					int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface.Name,
				)
			})
			if err != nil {
				return err
			}
			return controlErr
		},
	}

	transport := &http.Transport{
		DialContext: dialer.DialContext,
	}

	client := &http.Client{Transport: transport}

	// Make a new HTTP request to the metadata server.
	req, err := http.NewRequestWithContext(ctx, "GET", metadataServerURL, nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext(ctx, GET, %v, nil) failed: %v", metadataServerURL, err)
	}

	req.Header.Add("Metadata-Flavor", "Google")
	_, err = client.Do(req)
	return err
}
