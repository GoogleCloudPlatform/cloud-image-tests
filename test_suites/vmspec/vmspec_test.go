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

package vmspec

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// JSON representation of the ip link command output.
type ipLink struct {
	IfName    string
	OperState string
	MAC       string
	Flags     []string
}

const (
	interfaceNameLinuxFile   = "/var/tmp/interface_names.txt"
	interfaceNameWindowsFile = "C:\\interface_names.txt"
)

var (
	// Cache the interfaces so we don't have to run ip link between tests.
	interfaces []*ipLink
)

// This will write a file with the original names of the interfaces. The
// derivative VM will check that its interfaces do not match.
func TestEmpty(t *testing.T) {
	ctx := utils.Context(t)
	interfaces = parseInterfaceLinks(ctx, t)
	interfaceNameFile := interfaceNameLinuxFile
	if utils.IsWindows() {
		interfaceNameFile = interfaceNameWindowsFile
	}

	var names []string
	for _, iface := range interfaces {
		names = append(names, iface.MAC)
	}
	content := strings.Join(names, "\n")
	if err := os.WriteFile(interfaceNameFile, []byte(content), 0755); err != nil {
		t.Fatalf("error writing interface names to file: %v", err)
	}
}

// This tests that the PCIe configuration did change after a vmspec change.
func TestPCIEChanged(t *testing.T) {
	ctx := utils.Context(t)
	if len(interfaces) == 0 {
		interfaces = parseInterfaceLinks(ctx, t)
	}
	interfaceNameFile := interfaceNameLinuxFile
	if utils.IsWindows() {
		interfaceNameFile = interfaceNameWindowsFile
	}

	oldIfaceContent, err := os.ReadFile(interfaceNameFile)
	if err != nil {
		t.Fatalf("error reading interface names from file: %v", err)
	}
	oldIfaces := strings.Split(string(oldIfaceContent), "\n")

	if len(interfaces) == len(oldIfaces) {
		t.Fatalf("failed to change vmspec: same number of nics")
	}
}

// This is a simple test to ensure that network is functional. Most of the work
// is done by daisy for modifying the vmspec.
func TestPing(t *testing.T) {
	ctx := utils.Context(t)
	if len(interfaces) == 0 {
		interfaces = parseInterfaceLinks(ctx, t)
	}

	baseArgs := []string{
		// Ping 5 times with a 5 second timeout.
		"-c",
		"5",
		"-w",
		"5",
		"www.google.com",
	}
	nic := interfaces[0]
	pingArgs := append(baseArgs, "-I", nic.IfName)
	cmd := exec.CommandContext(ctx, "ping", pingArgs...)
	if res, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("error pinging google.com on nic %s: %v", nic, string(res))
	}
}

func TestWindowsPing(t *testing.T) {
	ctx := utils.Context(t)
	if len(interfaces) == 0 {
		interfaces = parseInterfaceLinks(ctx, t)
	}

	baseArgs := []string{
		// Ping 5 times with a 5 second timeout.
		"-n",
		"5",
		"-w",
		"5",
		"www.google.com",
	}

	for i, nic := range interfaces {
		iface, err := utils.GetInterfaceByMAC(nic.MAC)
		if err != nil {
			t.Fatalf("error getting interface %q: %v", nic.MAC, err)
		}
		ipAddr, err := parseInterfaceIpv4Addr(&iface)
		if err != nil {
			t.Fatalf("error getting %s ipv4 address: %v", nic, err)
		}
		pingArgs := append(baseArgs, "-S", ipAddr.String())

		cmd := exec.CommandContext(ctx, "ping", pingArgs...)
		if res, err := cmd.CombinedOutput(); err != nil && i == 0 {
			t.Fatalf("error pinging google.com on nic %s: %v", nic, string(res))
		} else if err == nil && i != 0 {
			t.Fatalf("unexpected success pinging google.com on nic %s", nic)
		}
	}
}

// Make sure that the metadata server is accessible from the primary NIC.
// TODO: Make sure secondary NICs cannot access MDS.
func TestMetadataServer(t *testing.T) {
	ctx := utils.Context(t)

	// Focus on primary NIC.
	nic := interfaces[0]
	httpClient := &http.Client{
		Timeout: 2 * time.Second,
	}

	// Make a new HTTP request to the metadata server.
	req, err := http.NewRequestWithContext(ctx, "GET", "http://metadata.google.internal/", nil)
	if err != nil {
		t.Fatalf("error creating request for %s: %v", nic, err)
	}

	// Get the interface by its name.
	iface, err := utils.GetInterfaceByMAC(nic.MAC)
	if err != nil {
		t.Fatalf("error getting interface %s: %v", nic, err)
	}

	// Obtain its IPv4 address.
	ipAddr, err := parseInterfaceIpv4Addr(&iface)
	if err != nil {
		t.Fatalf("error getting %s ipv4 address: %v", nic, err)
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
	if err != nil {
		t.Errorf("error connecting to metadata server on primary nic %s: %v", nic, err)
	}
}

// parseInterfaceLinks returns the names of all the interfaces on the system.
func parseInterfaceLinks(ctx context.Context, t *testing.T) []*ipLink {
	if utils.IsWindows() {
		res, err := utils.RunPowershellCmd("Get-NetAdapter -Name * -Physical | Format-Table -Property MacAddress -HideTableHeaders")
		if err != nil {
			t.Fatalf("error getting interface links: %v", res.Stderr)
		}

		var links []*ipLink
		for _, mac := range strings.Split(strings.TrimSpace(res.Stdout), "\r\n") {
			if mac == "" {
				continue
			}
			links = append(links, &ipLink{MAC: mac})
		}
		return links
	}
	out, err := exec.CommandContext(ctx, "ip", "-brief", "link").Output()
	if err != nil {
		stderr := string(err.(*exec.ExitError).Stderr)
		t.Fatalf("error getting interface names: %v", stderr)
	}
	fmt.Printf("Output: %v\n\n", string(out))

	var iflinks []*ipLink
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 4 {
			continue
		}

		fmt.Printf("Fields: %v\n", fields)

		iflink := &ipLink{
			IfName:    fields[0],
			OperState: fields[1],
			MAC:       fields[2],
			Flags:     strings.Split(strings.Trim(fields[3], "<>"), ","),
		}
		iflinks = append(iflinks, iflink)
	}

	// The first interface is the loopback interface, so leave it out.
	return iflinks[1:]
}

func parseInterfaceIpv4Addr(iface *net.Interface) (net.IP, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
			return ipnet.IP, nil
		}
	}
	return nil, fmt.Errorf("no ipv4 address found for interface %s", iface.Name)
}
