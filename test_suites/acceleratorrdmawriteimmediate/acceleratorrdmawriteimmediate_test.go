// Copyright 2024 Google LLC.
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

package acceleratorrdmawriteimmediate

import (
	"os/exec"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/acceleratorutils"
)

var writeWithImmediateArgs = []string{
	"--write_with_imm",
	"--size=64",
}

// Exercises the Write With Immediate RDMA verb using https://github.com/linux-rdma/perftest. This
// verb is is frequently used by NCCL to signal operation status.
// This is *not* a performance test, performance numbers aren't checked.
func TestWriteWithImmediateHost(t *testing.T) {
	ctx := utils.Context(t)
	acceleratorutils.SetupRDMAPerftestLibrary(ctx, t)
	command := exec.CommandContext(ctx, "./ib_write_lat", writeWithImmediateArgs...)
	out, err := command.CombinedOutput()
	t.Logf("%s output:\n%s", command, out)
	if err != nil {
		t.Fatalf("exec.CommandContext(%s).CombinedOutput() failed unexpectedly; err = %v", command, err)
	}
}

func TestWriteWithImmediateClient(t *testing.T) {
	ctx := utils.Context(t)
	acceleratorutils.SetupRDMAPerftestLibrary(ctx, t)
	acceleratorutils.RunRDMAClientCommand(ctx, t, "./ib_write_lat", writeWithImmediateArgs)
}
