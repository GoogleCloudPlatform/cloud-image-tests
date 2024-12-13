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
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	interfaceNameLinuxFile   = "/var/tmp/interface_names.txt"
	interfaceNameWindowsFile = "C:\\interface_names.txt"
)

var (
	// Cache the interfaces so we don't have to run ip link between tests.
	interfaces []net.Interface
)

// This will write a file with the original names of the interfaces. The
// derivative VM will check that its interfaces do not match.
func TestEmpty(t *testing.T) {
	interfaces = parseInterfaces(t)
	interfaceNameFile := interfaceNameLinuxFile
	if utils.IsWindows() {
		interfaceNameFile = interfaceNameWindowsFile
	}

	var names []string
	for _, iface := range interfaces {
		names = append(names, iface.Name)
	}
	content := strings.Join(names, "\n")
	if err := os.WriteFile(interfaceNameFile, []byte(content), 0755); err != nil {
		t.Fatalf("error writing interface names to file: %v", err)
	}
}

// This tests that the PCIe configuration did change after a vmspec change.
func TestPCIEChanged(t *testing.T) {
	if len(interfaces) == 0 {
		interfaces = parseInterfaces(t)
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
		t.Fatalf("failed to change vmspec: same number of nics, current:\n%v old:\n%v", interfaces, oldIfaces)
	}
}

// This is a simple test to ensure that network is functional. Most of the work
// is done by daisy for modifying the vmspec.
func TestPing(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	if len(interfaces) == 0 {
		interfaces = parseInterfaces(t)
	}

	baseArgs := []string{
		// Ping 5 times with a 5 second timeout.
		"-c",
		"5",
		"-w",
		"5",
		"www.google.com",
	}
	command := "ping"

	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	}
	if strings.Contains(image, "cos") {
		command = "toolbox"
		baseArgs = append([]string{"ping"}, baseArgs...)
	}

	nic := interfaces[0]
	pingArgs := append(baseArgs, "-I", nic.Name)
	cmd := exec.CommandContext(ctx, command, pingArgs...)
	if res, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("error pinging google.com on nic %s, result: %s, err: %v", nic.Name, string(res), err)
	}
}

func TestWindowsPing(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	if len(interfaces) == 0 {
		interfaces = parseInterfaces(t)
	}

	baseArgs := []string{
		// Ping 5 times with a 5 second timeout.
		"-n",
		"5",
		"-w",
		"5",
		"www.google.com",
	}

	// Focus on primary NIC.
	iface := interfaces[0]
	ipAddr, err := utils.ParseInterfaceIPv4(iface)
	if err != nil {
		t.Fatalf("error getting %s ipv4 address: %v", iface.Name, err)
	}
	pingArgs := append(baseArgs, "-S", ipAddr.String())

	cmd := exec.CommandContext(ctx, "ping", pingArgs...)
	if res, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("error pinging google.com on nic %s, result: %s, err: %v", iface.Name, string(res), err)
	}
}

// Make sure that the metadata server is accessible from the primary NIC.
// TODO: Make sure secondary NICs cannot access MDS.
func TestMetadataServer(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()

	// Focus on primary NIC.
	iface := interfaces[0]
	httpClient := &http.Client{
		Timeout: 2 * time.Second,
	}

	// Make a new HTTP request to the metadata server.
	req, err := http.NewRequestWithContext(ctx, "GET", "http://metadata.google.internal/", nil)
	if err != nil {
		t.Fatalf("error creating request for %s: %v", iface.Name, err)
	}

	// Obtain its IPv4 address.
	ipAddr, err := utils.ParseInterfaceIPv4(iface)
	if err != nil {
		t.Fatalf("error getting %s ipv4 address: %v", iface.Name, err)
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
		t.Errorf("error connecting to metadata server on primary nic %s: %v", iface.Name, err)
	}
}

// parseInterfaces returns the names of all the interfaces on the system.
func parseInterfaces(t *testing.T) []net.Interface {
	t.Helper()

	allIfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces() failed: %v", err)
	}

	return utils.FilterLoopbackTunnelingInterfaces(allIfaces)
}
