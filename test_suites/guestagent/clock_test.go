// Copyright 2025 Google LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     https://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package guestagent

import (
	"math"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func currHour(t *testing.T) (int, string) {
	t.Helper()
	output, err := exec.Command("date", "+%H").CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run date: %v, output: %s", err, string(output))
	}
	num, err := strconv.Atoi(strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatalf("Error converting string to int: %v", err)
	}
	output, err = exec.Command("date").CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run date: %v, output: %s", err, string(output))
	}
	return num, string(output)
}

func TestAgentNoClockDrift(t *testing.T) {
	utils.LinuxOnly(t)
	ctx := utils.Context(t)

	// Set rtc and timezone to local pst.
	output, err := exec.CommandContext(ctx, "timedatectl", "set-timezone", "America/Los_Angeles").CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to set timezone to pst: %v, output: %s", err, string(output))
	}
	t.Logf("Set timezone to pst: %v", string(output))

	beforeH, before := currHour(t)
	t.Logf("Current date output before setting rtc to local: %v", before)

	output, err = exec.CommandContext(ctx, "timedatectl", "set-local-rtc", "true").CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to override local rtc: %v, output: %s", err, string(output))
	}
	t.Logf("Set rtc to local: %v", string(output))

	if err := utils.RestartAgent(ctx); err != nil {
		t.Fatal(err)
	}

	// Add more delay to allow agent to confirm agent has run clock sync.
	time.Sleep(time.Second * 10)

	afterH, after := currHour(t)

	t.Logf("Current date output after setting rtc to local and restarting agent: %v", after)

	drift := math.Abs(float64(afterH - beforeH))
	if drift > 1 {
		t.Errorf("Clock drift detected: %v hours, before: %v, after: %v", drift, beforeH, afterH)
	}
}
