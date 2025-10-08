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

// Package wsfc is a CIT suite for testing Windows Server Failover Cluster
// functionality.
package wsfc

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"math/rand/v2"
)

const (
	tcpProtocol          = "tcp"
	numConnectionRetries = 3
	connectionRetryDelay = 1 * time.Second
)

// retryConnection retries the connection to the given destination host or VM
// until it succeeds or the maximum number of retries is reached.
func retryConnection(t *testing.T, protocol, addr string) (net.Conn, error) {
	t.Helper()

	for i := 0; i < numConnectionRetries; i++ {
		conn, err := net.Dial(protocol, addr)
		if err == nil {
			return conn, nil
		}
		t.Logf("retryConnection(%s, %s) attempt %d failed: %v, retrying in %v",
			protocol, addr, i, err, connectionRetryDelay)
		time.Sleep(connectionRetryDelay)
	}

	return nil, fmt.Errorf("failed to connect to %q after %d retries", addr, numConnectionRetries)
}

// sendReq sends a request [data] to the given address and returns response
// to it. The address must be in the format of "host:port" and the response
// will be in the format of "response\n".
func sendReq(t *testing.T, addr, data string) (string, error) {
	t.Helper()

	conn, err := retryConnection(t, tcpProtocol, addr)
	if err != nil {
		t.Fatalf("retryConnection(%s, %s) = %v, want nil", tcpProtocol, addr, err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(time.Second * 20))

	if n, err := fmt.Fprint(conn, data); err != nil || n != len(data) {
		t.Fatalf("conn.Write(%s) = wrote %d bytes, expected %d bytes, err: %v", data, n, len(data), err)
	}

	return bufio.NewReader(conn).ReadString('\n')
}

// randomIPv4 returns a random IPv4 address in the format of "1.2.3.4".
func randomIPv4() string {
	oct1 := rand.Int32()
	oct2 := rand.Int32()
	oct3 := rand.Int32()
	oct4 := rand.Int32()
	return fmt.Sprintf("%d.%d.%d.%d", oct1, oct2, oct3, oct4)
}

// validIPAddress returns the first valid IP address found setup on any
// nic interface.
func validIPAddress() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "0", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String(), nil
		}
	}

	return "", fmt.Errorf("no valid IP address found")
}

// TestHealthCheckAgent tests the health check agent by sending requests to
// the agent and verifying the responses.
func TestHealthCheckAgent(t *testing.T) {
	validIPAddress, err := validIPAddress()
	if err != nil {
		t.Fatalf("failed to get a valid interface IP: %v", err)
	}

	tests := []struct {
		desc string
		ip   string
		want string
	}{
		{
			desc: "unknown_ip",
			ip:   randomIPv4(),
			want: "0",
		},
		{
			desc: "valid_ip",
			ip:   validIPAddress,
			want: "1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			res, err := sendReq(t, fmt.Sprintf(":%s", wsfcAgentPort), tc.ip)

			if err != nil && err != io.EOF {
				t.Fatalf("sendReq failed: %v", err)
			}

			if len(res) == 0 {
				t.Errorf("sendReq(%s) returned empty response, want non-empty response", tc.ip)
			}

			if res != tc.want {
				t.Errorf("sendReq(%s) returned %q, want %q", tc.ip, res, tc.want)
			}
		})
	}
}
