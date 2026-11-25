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
	"net/netip"
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

	actualIPs, err := utils.GetGoogleRoutes(ctx, t, iface)
	if err != nil {
		return err
	}
	if err := verifyIPExist(ctx, actualIPs); err != nil {
		return err
	}
	return nil
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

	ctx := utils.Context(t)
	iface := readNic(ctx, t, 0)

	beforeRestart, err := utils.GetGoogleRoutes(ctx, t, iface)
	if err != nil {
		t.Fatal(err)
	}
	restartNetworkManager(ctx, t)

	// This is how long the guest agent can take to re-apply the routes, plus a
	// bit of padding to give time for the agent to re-add the routes.
	succeeded := true
	var afterRestart []string
	for i := 0; i < 2; i++ {
		time.Sleep(65 * time.Second)
		afterRestart, err = utils.GetGoogleRoutes(ctx, t, iface)
		if err != nil {
			t.Fatal(err)
		}
		if !compare(beforeRestart, afterRestart) {
			succeeded = false
			t.Logf("Failed attempt #%d: routes are inconsistent after restart, before %v after %v", i, beforeRestart, afterRestart)
		} else {
			return
		}
	}
	if !succeeded {
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
	beforeTrigger, err := utils.GetGoogleRoutes(ctx, t, iface)
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

	beforeTriggerV6, err := utils.GetGoogleIPv6Routes(ctx, t, iface)
	if err != nil {
		t.Fatalf("Failed to get Google IPv6 routes before ggactl trigger: %v", err)
	}
	for _, route := range beforeTriggerV6 {
		args := fmt.Sprintf("-6 route delete to local %s dev %s proto 66", route, iface.Name)
		cmd := exec.Command("ip", strings.Split(args, " ")...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to delete IPv6 route: %v, output: %q", err, string(out))
		}
	}

	out, err := exec.CommandContext(ctx, "ggactl_plugin", "routes", "setup").CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run ggactl_plugin routes setup: %v, output:\n %s", err, string(out))
	}

	afterTrigger, err := utils.GetGoogleRoutes(ctx, t, iface)
	if err != nil {
		t.Fatalf("Failed to get Google routes after ggactl trigger: %v", err)
	}

	if !compare(beforeTrigger, afterTrigger) {
		t.Fatalf("routes are inconsistent after ggactl trigger, before %v after %v", beforeTrigger, afterTrigger)
	}

	afterTriggerV6, err := utils.GetGoogleIPv6Routes(ctx, t, iface)
	if err != nil {
		t.Fatalf("Failed to get Google IPv6 routes after ggactl trigger: %v", err)
	}
	if !compare(beforeTriggerV6, afterTriggerV6) {
		t.Fatalf("IPv6 routes are inconsistent after ggactl trigger, before %v after %v", beforeTriggerV6, afterTriggerV6)
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

	beforeRestart, err := utils.GetGoogleRoutes(ctx, t, iface)
	if err != nil {
		t.Fatal(err)
	}
	if err := utils.RestartAgent(ctx); err != nil {
		t.Fatal(err)
	}

	afterRestart, err := utils.GetGoogleRoutes(ctx, t, iface)
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
	if strings.Contains(image, "guest-agent-stable") || strings.Contains(image, "ubuntu") || strings.Contains(image, "sles") {
		t.Skipf("Skipping test as it is not expected to pass on previous version of guest agent (image: %s).", image)
	}

	iface := readNic(ctx, t, 0)

	t.Cleanup(func() {
		// Swap the IP forwarding configuration from false to true.
		swapIPForwardingConfiguration(t, "ip_forwarding = false")

		if err := utils.RestartAgent(ctx); err != nil {
			t.Fatalf("Failed to restart agent after setting IP forwarding to true: %v", err)
		}
	})

	beforeRestart, err := utils.GetGoogleRoutes(ctx, t, iface)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Routes before restart: %v", beforeRestart)

	// Swap the IP forwarding configuration from true to false.
	swapIPForwardingConfiguration(t, "ip_forwarding = true")

	if err := utils.RestartAgent(ctx); err != nil {
		t.Fatalf("Failed to restart agent after setting IP forwarding to false: %v", err)
	}

	time.Sleep(10 * time.Second) // Give some extra time for the agent to do its job.

	afterRestart, err := utils.GetGoogleRoutes(ctx, t, iface)
	if err != nil {
		t.Fatalf("Error getting routes after restart: %v", err)
	}
	if len(afterRestart) != 0 {
		t.Fatalf("Routes [%s] exists after restart, but should not exist", strings.Join(afterRestart, ", "))
	}

	if compare(beforeRestart, afterRestart) {
		t.Fatalf("Routes are consistent after restart, but should not be. Before\n: %v, \nAfter\n: %v", beforeRestart, afterRestart)
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

	var changedConfig string

	if !strings.Contains(string(data), from) {
		changedConfig = fmt.Sprintf("%s\n[NetworkInterfaces]\n%s\n", string(data), to)
	} else {
		changedConfig = strings.ReplaceAll(string(data), from, to)
	}

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
	return fmt.Errorf("alias ip %s does not exist after reboot, routes: %v", expected, routes)
}

func TestIPv6Aliases(t *testing.T) {
	expected := readFirstIPv6AliasOrSkip(utils.Context(t), t)
	if err := verifyIPv6Aliases(t, expected); err != nil {
		t.Fatal(err)
	}
}

func TestIPv6AliasAfterReboot(t *testing.T) {
	expected := readFirstIPv6AliasOrSkip(utils.Context(t), t)
	if err := verifyIPv6Aliases(t, expected); err != nil {
		t.Fatal(err)
	}
}

func TestIPv6AliasAgentRestart(t *testing.T) {
	ctx := utils.Context(t)
	expected := readFirstIPv6AliasOrSkip(ctx, t)
	iface := readNic(ctx, t, 0)

	beforeRestart, err := utils.GetGoogleIPv6Routes(ctx, t, iface)
	if err != nil {
		t.Fatal(err)
	}
	if err := utils.RestartAgent(ctx); err != nil {
		t.Fatal(err)
	}

	afterRestart, err := utils.GetGoogleIPv6Routes(ctx, t, iface)
	if err != nil {
		t.Fatal(err)
	}
	if !compare(beforeRestart, afterRestart) {
		t.Fatalf("IPv6 routes are inconsistent after restart, before %v after %v", beforeRestart, afterRestart)
	}
	if err := verifyIPv6Exist(expected, afterRestart); err != nil {
		t.Fatal(err)
	}
}

func TestIPv6AliasAgentRestartWithIPv6AliasesConfigFalse(t *testing.T) {
	utils.LinuxOnly(t)
	ctx := utils.Context(t)
	_ = readFirstIPv6AliasOrSkip(ctx, t)
	iface := readNic(ctx, t, 0)

	configFile := "/etc/default/instance_configs.cfg"
	origData, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.WriteFile(configFile, origData, 0644); err != nil {
			t.Fatalf("Failed to restore %s: %v", configFile, err)
		}
		if err := utils.RestartAgent(ctx); err != nil {
			t.Fatalf("Failed to restart agent after restoring config: %v", err)
		}
	})

	updated := fmt.Sprintf("%s\n[IpForwarding]\nipv6_aliases = false\n", string(origData))
	if err := os.WriteFile(configFile, []byte(updated), 0644); err != nil {
		t.Fatal(err)
	}
	if err := utils.RestartAgent(ctx); err != nil {
		t.Fatalf("Failed to restart agent after setting ipv6_aliases = false: %v", err)
	}

	time.Sleep(10 * time.Second)

	afterRestartV6, err := utils.GetGoogleIPv6Routes(ctx, t, iface)
	if err != nil {
		t.Fatalf("Error getting IPv6 routes after restart: %v", err)
	}
	if len(afterRestartV6) != 0 {
		t.Fatalf("IPv6 alias routes %v exist after restart with ipv6_aliases = false, want none", afterRestartV6)
	}

	afterRestartV4, err := utils.GetGoogleRoutes(ctx, t, iface)
	if err != nil {
		t.Fatalf("Error getting IPv4 routes after restart: %v", err)
	}
	if err := verifyIPExist(ctx, afterRestartV4); err != nil {
		t.Fatalf("IPv4 alias routes should remain intact when only ipv6_aliases = false: %v", err)
	}
}

func readFirstIPv6AliasOrSkip(ctx context.Context, t *testing.T) string {
	t.Helper()
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("could not determine image: %v", err)
	}
	if utils.IsCOS(image) {
		t.Skipf("COS does not support IP aliases")
	}
	expected, err := utils.GetMetadata(ctx, "instance", "network-interfaces", "0", "ipv6-aliases", "0")
	if err != nil || strings.TrimSpace(expected) == "" {
		t.Skipf("no IPv6 alias ranges configured in metadata: %v", err)
	}
	return strings.TrimSpace(expected)
}

func verifyIPv6Aliases(t *testing.T, expected string) error {
	ctx := utils.Context(t)
	iface, err := utils.GetInterface(ctx, 0)
	if err != nil {
		return fmt.Errorf("couldn't get interface: %v", err)
	}

	actualIPs, err := utils.GetGoogleIPv6Routes(ctx, t, iface)
	if err != nil {
		return err
	}
	return verifyIPv6Exist(expected, actualIPs)
}

func verifyIPv6Exist(expected string, routes []string) error {
	expectedPrefix, err := parseIPv6Prefix(expected)
	if err != nil {
		return fmt.Errorf("failed to parse expected IPv6 alias %q: %v", expected, err)
	}
	for _, route := range routes {
		routePrefix, err := parseIPv6Prefix(route)
		if err != nil {
			continue
		}
		if routePrefix == expectedPrefix {
			return nil
		}
	}
	return fmt.Errorf("ipv6 alias %s does not exist, routes: %v", expected, routes)
}

func parseIPv6Prefix(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if prefix, err := netip.ParsePrefix(s); err == nil {
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(addr, 128), nil
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
