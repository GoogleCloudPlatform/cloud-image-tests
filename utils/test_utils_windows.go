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

//go:build windows

package utils

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
)

// GetGoogleRoutes returns the routes added by the guest agent.
func GetGoogleRoutes(ctx context.Context, t *testing.T, networkInterface net.Interface) ([]string, error) {
	status, err := RunPowershellCmd(fmt.Sprintf("Get-NetRoute -InterfaceIndex %d | ForEach-Object { Write-Output $_.DestinationPrefix }", networkInterface.Index))
	if err != nil {
		return nil, fmt.Errorf("failed to get routes: %v", err)
	}
	if status.Exitcode != 0 {
		return nil, fmt.Errorf("failed to get routes: %v", status.Stderr)
	}
	var routes []string
	for _, line := range strings.Split(status.Stdout, "\r\n") {
		route := strings.TrimSpace(line)
		if route == "" {
			continue
		}
		routes = append(routes, route)
	}
	return routes, nil
}
