// Copyright 2023 Google LLC
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
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func snapshotTestPrep(ctx context.Context, t *testing.T, runner *snapshotRunner) {
	t.Helper()
	if runner.isWindows {
		if err := runner.prepareWindowsSnapshot(ctx, t.Logf); err != nil {
			t.Fatalf("Windows snapshot preparation failed: %v", err)
		}
		return
	}

	// Make snapshots directory and write pre and post snapshot scripts.
	err := os.MkdirAll("/etc/google/snapshots", 0770)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("/etc/google/snapshots/pre.sh", []byte("#!/bin/bash\ndate>>/etc/google/snapshots/pre-snapshot-write\n"), 0770)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile("/etc/google/snapshots/post.sh", []byte("#!/bin/bash\ndate>>/etc/google/snapshots/post-snapshot-write\n"), 0770)
	if err != nil {
		t.Fatal(err)
	}

	// Enable snapshot scripts in the agent and restart it.
	// Wait for connection to snapshot service.
	agentcfg, err := os.ReadFile("/etc/default/instance_configs.cfg")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	// File may not end with a newline, so prepend one to make sure config is
	// valid.
	agentcfg = append(agentcfg, []byte("\n\n[Snapshots]\nenabled = true\ntimeout_in_seconds = 300\n")...)
	err = os.WriteFile("/etc/default/instance_configs.cfg", agentcfg, 0640)
	if err != nil {
		t.Fatal(err)
	}
	if err := utils.RestartAgent(ctx); err != nil {
		t.Fatal(err)
	}
	// Adds arbitrary buffer time for the agent to run late stage snapshot
	// module that runs after systemd-notify --ready.
	time.Sleep(30 * time.Second)
}

func verifySnapshotSuccess(ctx context.Context, t *testing.T, runner *snapshotRunner, sinceUTC string) {
	t.Helper()
	if runner.isWindows {
		// We have no way to communicate with VSSAgent, and polling for the
		// existence of shadows is flakier than checking its operational logs for
		// the winning snapshot attempt.
		missing, err := runner.verifyWindowsSnapshotLogs(ctx, sinceUTC, t.Logf)
		if err != nil {
			t.Fatal(err)
		}
		for _, msg := range missing {
			t.Errorf("Could not find message %q in GCE-VSS-Agent logs (since %q)", msg, sinceUTC)
		}
		return
	}

	// Read pre and post snapshot files; if they don't exist or weren't written
	// to once then the script execution did something unexpected.
	pre, err := os.ReadFile("/etc/google/snapshots/pre-snapshot-write")
	if err != nil {
		t.Fatal(err)
	}
	post, err := os.ReadFile("/etc/google/snapshots/post-snapshot-write")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(pre), "\n"); got != 1 {
		t.Errorf("Unexpected number of executions of /etc/google/snapshots/pre.sh: got %d, want 1", got)
	}
	if got := strings.Count(string(post), "\n"); got != 1 {
		t.Errorf("Unexpected number of executions of /etc/google/snapshots/post.sh: got %d, want 1", got)
	}
}

func TestSnapshotScripts(t *testing.T) {
	ctx := utils.Context(t)
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("Couldn't get image from metadata %v", err)
	}

	if utils.IsWindowsClient(image) {
		t.Skip("Skipping test on Windows client")
	}

	runner := newSnapshotRunner()
	snapshotTestPrep(ctx, t, runner)

	prj, zone, err := utils.GetProjectZone(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := utils.GetMetadata(ctx, "instance", "name")
	if err != nil {
		t.Fatal(err)
	}

	// Make a snapshot request for the boot disk of this instance.
	client, err := utils.GetDaisyClient(ctx)
	if err != nil {
		t.Fatalf("could not make compute api client: %v", err)
	}

	attemptStartUTC, _, err := runner.createSnapshotWithRetry(ctx, client, prj, zone, inst, t.Logf)
	if err != nil {
		t.Fatalf("unable to create snapshot: %v", err)
	}

	err = client.DeleteSnapshot(prj, "snapshot-"+inst)
	if err != nil {
		t.Errorf("unable to delete snapshot: %v", err)
	}

	verifySnapshotSuccess(ctx, t, runner, attemptStartUTC)
}
