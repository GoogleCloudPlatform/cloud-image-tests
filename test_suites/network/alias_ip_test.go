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

package network

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	markerFile = "/var/boot-marker"
)

func TestAliases(t *testing.T) {
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	}
	if strings.Contains(image, "cos") {
		t.Skipf("COS does not support IP aliases")
	}
	if err := verifyIPAliases(t); err != nil {
		t.Fatal(err)
	}
}

func TestAliasAfterReboot(t *testing.T) {
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	}
	if strings.Contains(image, "cos") {
		t.Skipf("COS does not support IP aliases")
	}
	_, err = os.Stat(markerFile)
	if os.IsNotExist(err) {
		// first boot
		if _, err := os.Create(markerFile); err != nil {
			t.Fatalf("failed creating marker file: %v", err)
		}
	} else if err != nil {
		t.Fatalf("failed to stat marker file: %+v", err)
	}

	// second boot
	if err := verifyIPAliases(t); err != nil {
		t.Fatal(err)
	}
}

func verifyIPAliases(t *testing.T) error {
	ctx := utils.Context(t)
	iface, err := utils.GetInterface(ctx, 0)
	if err != nil {
		return fmt.Errorf("couldn't get interface: %v", err)
	}

	actualIPs, err := getGoogleRoutes(iface.Name)
	if err != nil {
		return err
	}
	if err := verifyIPExist(ctx, actualIPs); err != nil {
		return err
	}
	return nil
}

func getGoogleRoutes(networkInterface string) ([]string, error) {
	// First, we probably need to wait so the guest agent can add the
	// routes. If this is insufficient, we might need to add retries.
	time.Sleep(30 * time.Second)

	arguments := strings.Split(fmt.Sprintf("route list table local type local scope host dev %s proto 66", networkInterface), " ")
	cmd := exec.Command("ip", arguments...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error listing Google routes: %s", b)
	}
	if len(b) == 0 {
		return nil, fmt.Errorf("No Google routes found")
	}
	var res []string
	for _, line := range strings.Split(string(b), "\n") {
		ip := strings.Split(line, " ")
		if len(ip) >= 2 {
			res = append(res, ip[1])
		}
	}
	return res, nil
}

func TestAliasAgentRestart(t *testing.T) {
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	}
	if strings.Contains(image, "cos") {
		t.Skipf("COS does not support IP aliases")
	}
	iface, err := utils.GetInterface(ctx, 0)
	if err != nil {
		t.Fatalf("couldn't get interface: %v", err)
	}

	beforeRestart, err := getGoogleRoutes(iface.Name)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("systemctl", "restart", "google-guest-agent")
	err = cmd.Run()
	if err != nil {
		t.Fatal(err)
	}
	afterRestart, err := getGoogleRoutes(iface.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !compare(beforeRestart, afterRestart) {
		t.Fatalf("routes are inconsistent after restart, before %v after %v", beforeRestart, afterRestart)
	}
	if err := verifyIPExist(ctx, afterRestart); err != nil {
		t.Fatal(err)
	}
}

func verifyIPExist(ctx context.Context, routes []string) error {
	expected, err := utils.GetMetadata(ctx, "instance", "network-interfaces", "0", "ip-aliases", "0")
	if err != nil {
		return fmt.Errorf("couldn't get first alias IP from metadata: %v", err)
	}
	for _, route := range routes {
		if route == expected {
			return nil
		}
	}
	return fmt.Errorf("alias ip %s is not exist after reboot", expected)
}

func compare(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
