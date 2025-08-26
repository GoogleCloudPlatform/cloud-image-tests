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

//go:build linux

package nicsetup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/nicsetup/managers"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"google.golang.org/api/compute/v1"
)

const (
	// Metadata constants. Sometimes the ping VM takes longer to start than the
	// other VMs, so we need to retry getting the metadata for a period of time
	// to make sure we give enough time for the ping VM to start up and set its
	// metadata.

	// numMetadataRetries is the number of times to retry getting metadata.
	numMetadataRetries = 30
	// metadataRetryDelay is the delay between metadata retries.
	metadataRetryDelay = time.Second * 10

	// Connection constants. These are set lower because the connections are only
	// attempted after we get a valid IPv4 and IPv6 address to which to connect.
	// So we don't need to wait as long for retries.

	// Retry the connection to ensure the destination is reachable.
	numConnectionRetries = 5
	// connectionRetryDelay is the delay between connection retries.
	connectionRetryDelay = time.Second
	// connectionTimeout is the timeout for connection attempts.
	connectionTimeout = time.Second

	// tcpTimeout is the timeout for TCP connections. We need to give ample time
	// for all pinging VMs to start up and connect to the ping VM.
	tcpTimeout = time.Minute * 10
	// expectedConnections is the number of connections we expect to receive for
	// each of IPv4 and IPv6. This number is 2/3 of the number of multiNIC Vms.
	expectedConnections = 6

	// ipv6AddrKey is the key containing the IPv6 address of the ping VM.
	ipv6AddrKey = "ipv6_addr"
	// pingVMPort is the port to listen on for the ping VM. This is chosen
	// arbitrarily to avoid conflicts with other services.
	pingVMPort = 1234
)

var (
	// numIPv4Connections is the number of IPv4 connections received.
	numIPv4Connections = 0
	ipv4ConnectionsMu  sync.Mutex
	// numIPv6Connections is the number of IPv6 connections received.
	numIPv6Connections = 0
	ipv6ConnectionsMu  sync.Mutex
	// expectedIpv4Connections is the number of IPv4 connections we expect to
	// receive. By default, we assume an amount equal to if IPv6 is not supported.
	expectedIPv4Connections = 1
	// expectedIPv6Connections is the number of IPv6 connections we expect to
	// receive. By default, we assume an amount equal to if IPv6 is not supported.
	expectedIPv6Connections = 0
)

// addressEntry is a representation of a single NIC entry in the `ip address` output.
type addressEntry struct {
	// IfIndex is the index of the interface.
	IfIndex int `json:"ifindex"`
	// IfName is the name of the interface.
	IfName string `json:"ifname"`
	// OperState is the operational state of the interface.
	OperState string `json:"operstate"`
	// Address is the MAC address of the interface.
	Address string `json:"address"`
	// Flags is the flags of the interface, like BROADCAST, MULTICAST, etc.
	Flags []string `json:"flags"`
	// AddrInfo is the address information of the interface.
	AddrInfo []addressInfo `json:"addr_info"`
}

// addressInfo represents the address information of a single NIC. A NIC can
// have multiple address information entries.
type addressInfo struct {
	// Family is the family of the address, like inet or inet6 for IPv4 and IPv6, respectively.
	Family string `json:"family"`
	// Local is the local IP address of the interface in the network.
	Local string `json:"local"`
	// Scope is the scope of the address, like global or link.
	Scope string `json:"scope"`
}

// getIPAddressOutput gets the `ip address` output. This is used to both verify
// the address entries for each NIC and for the ping VM to get its IPv6 address.
func getIPAddressOutput(t *testing.T) ([]addressEntry, error) {
	t.Helper()

	// Check the `ip address` output.
	out, err := exec.Command("ip", "--json", "address").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run `ip address`: %v", utils.ParseStderr(err))
	}

	// Unmarshal the output.
	var addressEntries []addressEntry
	if err := json.Unmarshal(out, &addressEntries); err != nil {
		return nil, fmt.Errorf("failed to unmarshal `ip address` output: %v", err)
	}
	return addressEntries, nil
}

// verifyConnection verifies that the given NIC has the proper connection.
//
// This first checks the `ip address` output to make sure that the NIC is up
// and has the proper addresses assigned to it according to its stack type.
// Then it creates a connection to a destination host or VM depending on
// whether the NIC is the primary or secondary NIC, respectively.
func verifyConnection(t *testing.T, nic managers.EthernetInterface) {
	t.Helper()
	t.Logf("%s: Verifying connection for NIC %q", getCurrentTime(), nic.Name)

	// Check the `ip address` output.
	addressEntries, addrError := getIPAddressOutput(t)

	// Check the address entries. Only check if the address entries command ran
	// successfully.
	if addrError == nil {
		var passed bool
		for _, entry := range addressEntries {
			if entry.IfName == nic.Name {
				// Make sure the interface is UP.
				if entry.OperState != "UP" {
					t.Errorf("NIC %q does is not UP, state is %q", nic.Name, entry.OperState)
				}

				// Check whether the interface has global IPv4 and IPv6 addresses.
				var hasIpv4, hasIpv6 bool
				for _, info := range entry.AddrInfo {
					// Global scope has external connectivity.
					if info.Scope == "global" {
						// inet family is IPv4.
						if info.Family == "inet" {
							if net.ParseIP(info.Local).To4() == nil {
								t.Errorf("NIC %q has invalid IPv4 address %q", nic.Name, info.Local)
							}
							hasIpv4 = true
							nic.IPv4Address = info.Local
						}
						// inet6 family is IPv6.
						if info.Family == "inet6" {
							if net.ParseIP(info.Local).To4() != nil {
								t.Errorf("NIC %q has invalid IPv6 address %q", nic.Name, info.Local)
							}
							hasIpv6 = true
							nic.IPv6Address = info.Local
						}
					}
				}
				if nic.StackType&managers.Ipv4 != 0 && !hasIpv4 {
					t.Errorf("NIC %q does not have IPv4 address", nic.Name)
				}
				if nic.StackType&managers.Ipv6 != 0 && !hasIpv6 {
					t.Errorf("NIC %q does not have IPv6 address", nic.Name)
				}
				passed = true
				break
			}
		}
		if !passed {
			t.Fatalf("NIC %q not found in `ip address` output", nic.Name)
		}
	} else {
		t.Logf("Skipping ip address check for NIC %q: %v", nic.Name, addrError)
	}

	if nic.Index == 0 {
		// Primary NIC should have a connection to the ping VM.
		connectionHost(t, nic, "www.google.com")
	} else {
		// Secondary NIC should have a connection to the host.
		connectionPing(t, nic)
	}
}

// connectionPing creates a connection to the ping VM using the given NIC.
//
// The ping VM has a static internal IPv4 address, and we can get the IPv6
// address from its metadata attributes. This step hangs until it is able to
// get the IPv6 address from the metadata.
func connectionPing(t *testing.T, nic managers.EthernetInterface) {
	t.Helper()
	t.Logf("%s: Getting ping VM parameters", getCurrentTime())

	pingVM, err := utils.GetRealVMName(utils.Context(t), "ping")
	if err != nil {
		t.Fatalf("Failed to get ping VM name: %v", err)
	}

	var ipv6Addr string
	// Only attempt to fetch the IPv6 address from the metadata if the image supports IPv6.
	if supportsIpv6 {
		for i := 0; i < numMetadataRetries; i++ {
			metadata := utils.GetInstanceMetadata(t, pingVM)
			for _, item := range metadata.Items {
				if item.Key == ipv6AddrKey {
					ipv6Addr = *item.Value
					break
				}
			}
			if ipv6Addr != "" {
				break
			}
			t.Logf("%s: Failed to get IPv6 address from metadata, retrying in %v", getCurrentTime(), metadataRetryDelay)
			time.Sleep(metadataRetryDelay)
		}
	}

	createConnection(t, nic, pingVMIPv4, ipv6Addr, strconv.Itoa(pingVMPort))
}

// connectionHost creates a connection to the given host using the given NIC.
//
// This attempts to resolve the host to an IPv4 and IPv6 address, and then
// creates a connection to the host on port 443 (HTTPS).
func connectionHost(t *testing.T, nic managers.EthernetInterface, host string) {
	t.Helper()

	// Attempt to resolve host.
	addrs, err := net.LookupHost(host)
	if err != nil {
		t.Fatalf("failed to resolve host %q: %v", host, err)
	}

	// Find the first IPv4 and IPv6 addresses in the list of addresses.
	var ipv4Addr, ipv6Addr string
	for _, addr := range addrs {
		if net.ParseIP(addr).To4() != nil {
			if ipv4Addr == "" {
				ipv4Addr = addr
			}
		} else {
			if ipv6Addr == "" {
				ipv6Addr = addr
			}
		}
	}

	// Port 443 is for HTTPS.
	t.Logf("Resolved host %q to IPv4 %q and IPv6 %q", host, ipv4Addr, ipv6Addr)
	createConnection(t, nic, ipv4Addr, ipv6Addr, "443")
}

// createConnection creates a connection to the given host using the given NIC.
//
// This creates a dialer that is bound to the given NIC, and attempts to connect
// to the given IPv4 and IPv6 addresses on the given port. If the NIC supports
// IPv4, we expect to connect successfully to the IPv4 address. If the NIC
// supports IPv6, we expect to connect successfully to the IPv6 address.
// If it supports one but not the other, we expect to connect successfully to
// the address that is supported and unsuccessfully to the address that is
// not supported.
func createConnection(t *testing.T, nic managers.EthernetInterface, ipv4Addr, ipv6Addr, port string) {
	t.Helper()
	t.Logf("%[5]s: Creating connection to %[1]s:%[3]s and [%[2]s]:%[3]s via NIC %[4]q", ipv4Addr, ipv6Addr, port, nic.Name, getCurrentTime())

	dialer := &net.Dialer{
		Control: func(network, address string, c syscall.RawConn) error {
			var controlErr error
			err := c.Control(func(fd uintptr) {
				controlErr = syscall.SetsockoptString(
					int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, nic.Name,
				)
			})
			if err != nil {
				return err
			}
			return controlErr
		},
		Timeout: connectionTimeout,
	}

	// Attempt to connect via IPv4.
	ipv4Err := retryConnection(t, dialer, "tcp4", net.JoinHostPort(ipv4Addr, port))
	if nic.StackType&managers.Ipv4 != 0 {
		// We expect to connect successfully to the IPv4 address.
		if ipv4Err != nil {
			t.Errorf("failed to dial %q via NIC %q (IPv4): %v", ipv4Addr, nic.Name, ipv4Err)
		}
	} else {
		// We somehow connected to the IPv4 address when the NIC doesn't support IPv4.
		if ipv4Err == nil {
			t.Errorf("unexpected success dialing %q via NIC %q (IPv4)", ipv4Addr, nic.Name)
		}
	}

	// Attempt to connect via IPv6.
	ipv6Err := retryConnection(t, dialer, "tcp6", net.JoinHostPort(ipv6Addr, port))
	if nic.StackType&managers.Ipv6 != 0 {
		// We expect to connect successfully to the IPv6 address.
		if ipv6Err != nil {
			t.Errorf("failed to dial %q via NIC %q (IPv6): %v", ipv6Addr, nic.Name, ipv6Err)
		}
	} else {
		// We somehow connected to the IPv6 address when the NIC doesn't support IPv6.
		if ipv6Err == nil {
			t.Errorf("unexpected success dialing %q via NIC %q (IPv6)", ipv6Addr, nic.Name)
		}
	}

	t.Logf("%s: Finished creating connections via NIC %q", getCurrentTime(), nic.Name)
}

// retryConnection retries the connection to the given destination host or VM
// until it succeeds or the maximum number of retries is reached.
func retryConnection(t *testing.T, dialer *net.Dialer, protocol, dest string) error {
	t.Helper()
	t.Logf("%s: Dialing to %q via %q", getCurrentTime(), dest, protocol)

	var errs []error
	for i := 0; i < numConnectionRetries; i++ {
		timeoutCtx, cancel := context.WithTimeout(utils.Context(t), connectionTimeout)
		defer cancel()
		_, err := dialer.DialContext(timeoutCtx, protocol, dest)
		if err == nil {
			return nil
		}
		// Ignore context deadline exceeded errors.
		if !errors.Is(err, context.DeadlineExceeded) {
			errs = append(errs, err)
		}
		time.Sleep(connectionRetryDelay)
	}
	return fmt.Errorf("failed to connect to %q after %d retries: %w", dest, numConnectionRetries, errors.Join(errs...))
}

// testEmpty is a no-op test that sets up the ping VM for TCP connections and
// writes its IPv6 address to a metadata attribute. Other VMs in the test can
// then use this IPv6 address to try to connect to the TCP listener on the
// ping VM.
func testEmpty(t *testing.T) {
	t.Helper()

	// Get the IPv6 address of the ping VM. This is only needed if the image
	// supports IPv6.
	if supportsIpv6 {
		expectedIPv6Connections = expectedConnections
		expectedIPv4Connections = expectedConnections

		// Get the real name of this VM.
		name, err := utils.GetInstanceName(utils.Context(t))
		if err != nil {
			t.Fatalf("Failed to get ping VM name: %v", err)
		}
		metadata := utils.GetInstanceMetadata(t, name)

		var ipv6Addr string
		addressEntries, err := getIPAddressOutput(t)
		if err != nil {
			t.Fatalf("Failed to get `ip address` output: %v", err)
		}
		for _, entry := range addressEntries {
			for _, info := range entry.AddrInfo {
				if info.Family == "inet6" && info.Scope == "global" {
					if net.ParseIP(info.Local) == nil {
						t.Errorf("NIC %q has invalid IPv6 address %q", entry.IfName, info.Local)
					}
					ipv6Addr = info.Local
					break
				}
			}
		}
		t.Logf("Ping VM IPv6 address: %s", ipv6Addr)
		// Add and set the IPv6 address to the metadata of the ping VM.
		metadata.Items = append(metadata.Items, &compute.MetadataItems{
			Key:   ipv6AddrKey,
			Value: &ipv6Addr,
		})
		utils.SetInstanceMetadata(t, name, metadata)
	}

	// Start listening on the ping VM port. The other VMs should be able to connect
	// to this VM with the given port.
	ipv4Listener, err := net.ListenTCP("tcp4", &net.TCPAddr{Port: pingVMPort})
	if err != nil {
		t.Fatalf("Failed to listen on IPv4: %v", err)
	}
	ipv4Context, ipv4Cancel := context.WithCancel(utils.Context(t))
	defer ipv4Listener.Close()
	defer ipv4Cancel()

	hangCtx, hangCancel := context.WithTimeout(utils.Context(t), tcpTimeout)
	defer hangCancel()

	// Accept IPv4 connections on the ping VM port.
	go func() {
		t.Logf("%s: Starting IPv4 listener", getCurrentTime())
		for {
			select {
			case <-ipv4Context.Done():
				return
			default:
				c, err := ipv4Listener.AcceptTCP()
				if err != nil {
					if opErr, ok := err.(*net.OpError); !ok || !opErr.Timeout() {
						t.Logf("%s: Failed to accept IPv4 connection: %v", getCurrentTime(), err)
					}
				} else {
					go func(conn *net.TCPConn) {
						t.Logf("%s: Accepted IPv4 connection from %s", getCurrentTime(), conn.RemoteAddr().String())
						defer conn.Close()
						// Only count connections from within the network.
						if strings.HasPrefix(conn.RemoteAddr().String(), "10.0.0.") {
							ipv4ConnectionsMu.Lock()
							numIPv4Connections++
							if numIPv4Connections == expectedIPv4Connections {
								t.Logf("%s: Received expected number of IPv4 connections, closing IPv4 listener.", getCurrentTime())
								ipv4Cancel()
								ipv4Listener.Close()
							}
							ipv4ConnectionsMu.Unlock()
						}
					}(c)
				}
			}
		}
	}()
	// Accept IPv6 connections on the ping VM port.
	// Only start listening on IPv6 if the image supports IPv6.
	if supportsIpv6 {
		ipv6Listener, err := net.ListenTCP("tcp6", &net.TCPAddr{Port: pingVMPort})
		if err != nil {
			t.Fatalf("Failed to listen on IPv6: %v", err)
		}
		ipv6Context, ipv6Cancel := context.WithCancel(utils.Context(t))
		defer ipv6Cancel()
		defer ipv6Listener.Close()

		go func() {
			t.Logf("%s: Starting IPv6 listener", getCurrentTime())
			for {
				select {
				case <-ipv6Context.Done():
					return
				default:
					c, err := ipv6Listener.AcceptTCP()
					if err != nil {
						if opErr, ok := err.(*net.OpError); !ok || !opErr.Timeout() {
							t.Logf("%s: Failed to accept IPv6 connection: %v", getCurrentTime(), err)
						}
					} else {
						go func(conn *net.TCPConn) {
							t.Logf("%s: Accepted IPv6 connection from %s", getCurrentTime(), conn.RemoteAddr().String())
							defer conn.Close()
							ipv6ConnectionsMu.Lock()
							// We don't need to check if the connection is from within the network
							// because IPv6 is internal to the network only.
							numIPv6Connections++
							if numIPv6Connections == expectedIPv6Connections {
								t.Logf("%s: Received expected number of IPv6 connections, closing IPv6 listener.", getCurrentTime())
								ipv6Cancel()
								ipv6Listener.Close()
							}
							ipv6ConnectionsMu.Unlock()
						}(c)
					}
				}
			}
		}()
	}

	// Hang the main process until we receive the expected number of connections.
	// Without a separate hang context, this ensures that the TCP listeners will
	// continue running until the test times out, or until the expected number
	// of connections is received, whichever comes first.
	for {
		select {
		case <-hangCtx.Done():
			t.Logf("%s: Hanging context done, stopping", getCurrentTime())
			return
		default:
			ipv4ConnectionsMu.Lock()
			ipv6ConnectionsMu.Lock()
			// If we reached the expected number of connections for both IPv4 and IPv6,
			// then we can stop the test.
			if numIPv4Connections == expectedIPv4Connections && numIPv6Connections == expectedIPv6Connections {
				ipv4ConnectionsMu.Unlock()
				ipv6ConnectionsMu.Unlock()
				t.Logf("Received expected number of connections, stopping")
				return
			}
			ipv4ConnectionsMu.Unlock()
			ipv6ConnectionsMu.Unlock()
			time.Sleep(time.Second * 10)
		}
	}
}
