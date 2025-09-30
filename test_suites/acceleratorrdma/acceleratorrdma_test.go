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

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/acceleratorutils"
)

func buildIBWriteBWArgs(ctx context.Context, t *testing.T) []string {
	args := []string{
		"--report_gbits", // Generate report in Gb/s for human readability.
		"--use_cuda=0",   // Use the first cuda device.
		"--all",          // Test all message sizes.
		"--qp=200",       // 200 active Queue Pair operations. 200 was suggested by netinfra team.
	}
	image, err := utils.GetMetadata(ctx, "instance", "image")
	if err != nil {
		t.Fatalf("utils.GetMetadata(ctx, instance, image) = %v, want nil", err)
	}
	if strings.Contains(image, "rocky-linux-8") {
		// DMABuf support is too new for this kernel, setup peermem.
		out, err := exec.CommandContext(ctx, "modprobe", "nvidia-peermem").CombinedOutput()
		if err != nil {
			t.Fatalf("exec.CommandContext(ctx, modprobe, nvidia-peermem).CombinedOutput() = %v, want nil\noutput: %s", err, out)
		}
	} else {
		// DMABuf is supported, use it.
		args = append(args, "--use_cuda_dmabuf")
	}
	return args
}

// Exercise the GPUDirectRDMA stack without involving NCCL by using
// https://github.com/linux-rdma/perftest.
// This is *not* a performance test, performance numbers aren't checked.
func TestGPUDirectRDMAHost(t *testing.T) {
	ctx := utils.Context(t)
	acceleratorutils.SetupRDMAPerftestLibrary(ctx, t)
	ibWriteBWArgs := buildIBWriteBWArgs(ctx, t)
	ibWriteBWCmd := exec.CommandContext(ctx, "./ib_write_bw", ibWriteBWArgs...)
	out, err := ibWriteBWCmd.CombinedOutput()
	t.Logf("%s output:\n%s", ibWriteBWCmd.String(), out)
	if err != nil {
		t.Fatalf("exec.CommandContext(%s).CombinedOutput() = %v, want nil", ibWriteBWCmd.String(), err)
	}
}

func TestGPUDirectRDMAClient(t *testing.T) {
	ctx := utils.Context(t)
	acceleratorutils.SetupRDMAPerftestLibrary(ctx, t)
	ibWriteBWArgs := buildIBWriteBWArgs(ctx, t)
	acceleratorutils.RunRDMAClientCommand(ctx, t, "./ib_write_bw", ibWriteBWArgs)
}
