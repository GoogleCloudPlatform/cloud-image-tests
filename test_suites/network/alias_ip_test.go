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
	"errors"
	"fmt"
	"net"
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
	if utils.IsCOS(image) {
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
	if utils.IsCOS(image) {
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

func skipIfUbuntu(t *testing.T) {
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	}
	if utils.IsUbuntu(image) {
		t.Skipf("Skipping test for Ubuntu images due to b/434210587")
	}
}

func TestNetworkManagerRestart(t *testing.T) {
	utils.LinuxOnly(t)
	// TODO(b/434210587): Remove this skip once the bug is fixed.
	skipIfUbuntu(t)
	ctx := utils.Context(t)
	iface := readNic(ctx, t, 0)
	beforeRestart, err := getGoogleRoutes(iface.Name)
	if err != nil {
		t.Fatal(err)
	}
	restartNetworkManager(ctx, t)

	// This is how long the guest agent can take to re-apply the routes, plus a
	// bit of padding to give time for the agent to re-add the routes.
	time.Sleep(65 * time.Second)
	afterRestart, err := getGoogleRoutes(iface.Name)
	if err != nil {
		t.Fatal(err)
	}
	if !compare(beforeRestart, afterRestart) {
		t.Fatalf("routes are inconsistent after restart, before %v after %v", beforeRestart, afterRestart)
	}
}

func TestGgactlCommand(t *testing.T) {
	utils.LinuxOnly(t)
	ctx := utils.Context(t)
	if !utils.CheckLinuxCmdExists("ggactl_plugin") {
		t.Skipf("ggactl_plugin executable not found, skipping test")
	}

	iface := readNic(ctx, t, 0)
	beforeTrigger, err := getGoogleRoutes(iface.Name)
	if err != nil {
		t.Fatalf("Failed to get Google routes before ggactl trigger: %v", err)
	}
	if len(beforeTrigger) == 0 {
		t.Fatalf("No Google routes found before ggactl trigger")
	}

	for _, route := range beforeTrigger {
		args := fmt.Sprintf("route delete to local %s scope host dev %s proto 66", route, iface.Name)
		cmd := exec.Command("ip", strings.Split(args, " ")...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to delete route: %v, output: %q", err, string(out))
		}
	}

	out, err := exec.CommandContext(ctx, "ggactl_plugin", "routes", "setup").CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run ggactl_plugin routes setup: %v, output:\n %s", err, string(out))
	}

	afterTrigger, err := getGoogleRoutes(iface.Name)
	if err != nil {
		t.Fatalf("Failed to get Google routes after ggactl trigger: %v", err)
	}

	if !compare(beforeTrigger, afterTrigger) {
		t.Fatalf("routes are inconsistent after ggactl trigger, before %v after %v", beforeTrigger, afterTrigger)
	}
}

func restartNetworkManager(ctx context.Context, t *testing.T) {
	t.Helper()
	// SLES skips ip alias tests.
	managers := []string{"systemd-networkd", "NetworkManager"}
	var allErrs error
	for _, manager := range managers {
		cmd := exec.CommandContext(ctx, "systemctl", "restart", manager)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return
		}
		allErrs = errors.Join(allErrs, fmt.Errorf("failed to restart %q: %s, err: %v", manager, string(out), err))
	}
	t.Skipf("No known network manager found, skipping test: %v", allErrs)
}

func readNic(ctx context.Context, t *testing.T, id int) net.Interface {
	t.Helper()
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	}
	if utils.IsCOS(image) {
		t.Skipf("COS does not support IP aliases")
	}
	iface, err := utils.GetInterface(ctx, id)
	if err != nil {
		t.Fatalf("couldn't get interface: %v", err)
	}
	return iface
}

func TestAliasAgentRestart(t *testing.T) {
	ctx := utils.Context(t)
	iface := readNic(ctx, t, 0)
	beforeRestart, err := getGoogleRoutes(iface.Name)
	if err != nil {
		t.Fatal(err)
	}
	if err := utils.RestartAgent(ctx); err != nil {
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

func TestAliasAgentRestartWithIPForwardingConfigFalse(t *testing.T) {
	utils.LinuxOnly(t)
	ctx := utils.Context(t)

	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	}

	// TODO(b/448377923): Remove this skip once the agent is released.
	if strings.Contains(image, "guest-agent-stable") || !strings.Contains(image, "guest-agent") {
		t.Skipf("Skipping test as it is not expected to pass on previous version of guest agent (image: %s).", image)
	}

	iface := readNic(ctx, t, 0)

	t.Cleanup(func() {
		// Swap the IP forwarding configuration from false to true.
		swapIPForwardingConfiguration(t, "ip_forwarding = false")

		if err := utils.RestartAgent(ctx); err != nil {
			t.Fatal(err)
		}
	})

	beforeRestart, err := getGoogleRoutes(iface.Name)
	if err != nil {
		t.Fatal(err)
	}

	// Swap the IP forwarding configuration from true to false.
	swapIPForwardingConfiguration(t, "ip_forwarding = true")

	if err := utils.RestartAgent(ctx); err != nil {
		t.Fatal(err)
	}

	afterRestart, err := getGoogleRoutes(iface.Name)
	if err == nil {
		t.Fatal("Routes exists after restart, but should not exist")
	}

	if compare(beforeRestart, afterRestart) {
		t.Fatalf("Routes are consistent after restart, but should not be")
	}
}

func swapIPForwardingConfiguration(t *testing.T, currentConfig string) {
	t.Helper()

	configFile := "/etc/default/instance_configs.cfg"

	toggle := map[string]string{
		"ip_forwarding = true":  "ip_forwarding = false",
		"ip_forwarding = false": "ip_forwarding = true",
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}

	from := currentConfig
	to := toggle[currentConfig]

	changedConfig := strings.ReplaceAll(string(data), from, to)

	if err := os.WriteFile(configFile, []byte(changedConfig), 0644); err != nil {
		t.Fatal(err)
	}
}

func verifyIPExist(ctx context.Context, routes []string) error {
	expected, err := utils.GetMetadata(ctx, "instance", "network-interfaces", "0", "ip-aliases", "0")
	if err != nil {
		return fmt.Errorf("couldn't get first alias IP from metadata: %v", err)
	}
	for _, route := range routes {
		if strings.Contains(expected, route) {
			return nil
		}
	}
	return fmt.Errorf("alias ip %s is not exist after reboot, routes: %v", expected, routes)
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
