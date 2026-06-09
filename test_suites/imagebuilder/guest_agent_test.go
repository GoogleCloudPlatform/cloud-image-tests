package imagebuilder

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// TestGoogleGuestAgentHealthy checks if the Google Guest Agent service is active and fully operational (SubState=running).
func TestGoogleGuestAgentHealthy(t *testing.T) {
	ctx := utils.Context(t)

	// 1. Check if the service is active.
	cmdActive := exec.CommandContext(ctx, "systemctl", "is-active", "google-guest-agent.service")
	stdoutActive, err := cmdActive.Output()
	if err != nil {
		t.Errorf("Running `systemctl is-active google-guest-agent.service` expected `active`, but got: %q, Error: %v",
			strings.TrimSpace(string(stdoutActive)), err)
	}

	// 2. Query systemd for the exact substate of the service manager.
	cmdShow := exec.CommandContext(ctx, "systemctl", "show", "google-guest-agent.service", "--property=SubState")
	stdoutShow, err := cmdShow.Output()
	if err != nil {
		t.Fatalf("Failed while running `systemctl show google-guest-agent.service --property=SubState` to query systemd for SubState: %v. Stderr: %q", err, utils.ParseStderr(err))
	}

	// Output format is usually "SubState=running" or "SubState=dead"
	substate := strings.TrimSpace(string(stdoutShow))
	if !strings.Contains(substate, "SubState=running") {
		t.Errorf("Health Check Failed: Agent is not fully operational after running 'systemctl show google-guest-agent.service --property=SubState'. Found %q (expected SubState=running)", substate)
	} else {
		t.Log("Health Check Passed: google-guest-agent is actively running.")
	}
}
