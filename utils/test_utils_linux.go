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

package utils

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"testing"
)

// GetGoogleRoutes returns the IPv4 routes added by the guest agent.
func GetGoogleRoutes(ctx context.Context, t *testing.T, networkInterface net.Interface) ([]string, error) {
	return getGoogleRoutes(ctx, t, networkInterface, false)
}

// GetGoogleIPv6Routes returns the IPv6 routes added by the guest agent.
func GetGoogleIPv6Routes(ctx context.Context, t *testing.T, networkInterface net.Interface) ([]string, error) {
	return getGoogleRoutes(ctx, t, networkInterface, true)
}

func getGoogleRoutes(ctx context.Context, t *testing.T, networkInterface net.Interface, ipv6 bool) ([]string, error) {
	var args []string
	if ipv6 {
		args = []string{"-6", "route", "list", "table", "local", "type", "local", "dev", networkInterface.Name, "proto", "66"}
	} else {
		args = []string{"route", "list", "table", "local", "type", "local", "scope", "host", "dev", networkInterface.Name, "proto", "66"}
	}
	cmd := exec.CommandContext(ctx, "ip", args...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error listing Google routes (%s), err: %v", b, err)
	}

	var res []string
	for _, line := range strings.Split(string(b), "\n") {
		ip := strings.Split(line, " ")
		if len(ip) >= 2 {
			route := ip[1]
			t.Logf("Found route %q", route)
			res = append(res, route)
		}
	}
	return res, nil
}
