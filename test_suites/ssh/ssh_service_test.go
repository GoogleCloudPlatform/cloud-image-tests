// Copyright 2026 Google LLC.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package ssh

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	guestAgentService        = "google-guest-agent.service"
	guestAgentManagerService = "google-guest-agent-manager.service"
)

// TestSSHServiceStartsAfterGuestAgent verifies that the SSH daemon
// marks itself as ready only after the Google Guest Agent is ready.
func TestSSHServiceStartsAfterGuestAgent(t *testing.T) {
	ctx := context.Background()

	// Determine which guest agent service is active and get its timestamps
	agentActiveService := guestAgentService
	agentReadyTime, err := getServiceTimestamps(ctx, agentActiveService)
	if err != nil {
		t.Logf("Failed to get timestamps for %s: %v. Trying %s...", agentActiveService, err, guestAgentManagerService)

		agentActiveService = guestAgentManagerService
		agentReadyTime, err = getServiceTimestamps(ctx, agentActiveService)
		if err != nil {
			t.Fatalf("Failed to get ready time for both %s and %s: %v", guestAgentService, guestAgentManagerService, err)
		}
	}
	t.Logf("%s ActiveEnterTimestamp: %v", agentActiveService, agentReadyTime)

	// Identify the correct SSH service name based on the OS family
	sshService := "sshd.service"
	if _, err := os.Stat("/lib/systemd/system/ssh.service"); err == nil {
		sshService = "ssh.service"
	}

	// Get the timestamps for the SSH service
	sshReadyTime, err := getServiceTimestamps(ctx, sshService)
	if err != nil {
		t.Fatalf("Failed to get ready time for %s: %v", sshService, err)
	}
	t.Logf("%s ActiveEnterTimestamp: %v", sshService, sshReadyTime)

	// Assert that SSH started after the agent
	if sshReadyTime.Before(agentReadyTime) {
		t.Errorf("SSH service (%s) became ready (%v) BEFORE Guest Agent (%s) became ready (%v).",
			sshService, sshReadyTime, agentActiveService, agentReadyTime)
	} else {
		t.Logf("PASS: SSH service (%s) became ready (%v) AFTER Guest Agent (%s) became ready (%v).",
			sshService, sshReadyTime, agentActiveService, agentReadyTime)
	}
}

// getServiceTimestamps queries systemd's DBus API to get the exact microsecond ActiveEnterTimestamp.
func getServiceTimestamps(ctx context.Context, service string) (time.Time, error) {
	// 1. Escape the service name for DBus object path routing
	// Example: "google-guest-agent.service" -> "google_2dguest_2dagent_2eservice"
	dbusPath := strings.ReplaceAll(service, "-", "_2d")
	dbusPath = strings.ReplaceAll(dbusPath, ".", "_2e")
	fullPath := fmt.Sprintf("/org/freedesktop/systemd1/unit/%s", dbusPath)

	// 2. Ask busctl for the raw microsecond timestamp
	cmd := exec.CommandContext(ctx, "busctl", "get-property", "org.freedesktop.systemd1",
		fullPath, "org.freedesktop.systemd1.Unit", "ActiveEnterTimestamp")
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, fmt.Errorf("busctl command failed for %s: %v\nOutput: %s", service, err, out)
	}

	// 3. Parse the output (busctl returns format: "t 1653139385938123")
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return time.Time{}, fmt.Errorf("unexpected busctl output format for %s: %s", service, string(out))
	}

	microSec, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse microseconds %q for %s: %v", fields[1], service, err)
	}

	if microSec == 0 {
		return time.Time{}, fmt.Errorf("service %s has not been started (timestamp is 0)", service)
	}

	return time.UnixMicro(microSec), nil
}
