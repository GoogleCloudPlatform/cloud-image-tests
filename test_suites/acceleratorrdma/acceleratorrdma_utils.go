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

package acceleratorrdma

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// runRDMAClientCommand executes a RDMA test command targeting the host VM. The host VM must run the
// same command. It retries on connection errors, as the client might be ready before the host.
func runRDMAClientCommand(ctx context.Context, t *testing.T, command string, args []string) {
	t.Helper()
	target, err := utils.GetRealVMName(ctx, rdmaHostName)
	if err != nil {
		t.Fatalf("utils.GetRealVMName(%s) = %v, want nil", rdmaHostName, err)
	}
	fullArgs := append(args, target)
	for {
		command := exec.CommandContext(ctx, command, fullArgs...)
		out, err := command.CombinedOutput()
		if err == nil {
			t.Logf("%s output:\n%s", command, out)
			return
		}
		// Client may be ready before host, retry connection errors.
		if strings.Contains(string(out), "Couldn't connect to "+target) {
			time.Sleep(time.Second)
			if ctx.Err() != nil {
				t.Logf("%s output:\n%s", command, out)
				t.Fatalf("context expired before connecting to host: %v\nlast %q error was: %v", ctx.Err(), command, err)
			}
			continue
		}

		t.Logf("%s output:\n%s", command, out)
		t.Fatalf("exec.CommandContext(%s).CombinedOutput() failed unexpectedly; err %v", command, err)
	}
}
