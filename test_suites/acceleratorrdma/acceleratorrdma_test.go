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
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func setupPerftest(ctx context.Context, t *testing.T) {
	t.Helper()
	switch {
	case utils.CheckLinuxCmdExists("yum"):
		out, err := exec.CommandContext(ctx, "yum", "install", "-y", "git", "cuda-toolkit", "perftest", "libtool", "automake", "autoconf", "make", "libibverbs-devel", "librdmacm-devel", "libibumad-devel", "pciutils-devel").CombinedOutput()
		if err != nil {
			t.Fatalf("exec.CommandContext(ctx, yum, install, -y, git, cuda-toolkit, perftest, libtool, automake, autoconf, make, libibverbs-devel, librdmacm-devel, libibumad-devel, pciutils-devel).CombinedOutput() = %v, want nil\noutput: %s", err, out)
		}
	case utils.CheckLinuxCmdExists("apt"):
		out, err := exec.CommandContext(ctx, "add-nvidia-repositories", "-y").CombinedOutput()
		if err != nil {
			t.Fatalf("exec.CommandContext(ctx, add-nvidia-repositories, -y).CombinedOutput() = %v, want nil\noutput: %s", err, out)
		}
		out, err = exec.CommandContext(ctx, "apt", "install", "-y", "git", "ibverbs-utils", "cuda-toolkit", "perftest", "libtool", "automake", "autoconf", "libibverbs-dev", "librdmacm-dev", "libibumad-dev", "libpci-dev", "make").CombinedOutput()
		if err != nil {
			t.Fatalf("exec.CommandContext(ctx, apt, install, -y, git, ibverbs-utils, cuda-toolkit, perftest, libtool, automake, autoconf, libibverbs-dev, librdmacm-dev, libibumad-dev, libpci-dev, make).CombinedOutput() = %v, want nil\noutput: %s", err, out)
		}
	default:
		t.Fatalf("Unknown package manager, can't install build deps.")
	}
	out, err := exec.CommandContext(ctx, "git", "clone", "--depth=1", "https://github.com/linux-rdma/perftest").CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, git, clone, --depth=1, https://github.com/linux-rdma/perftest).CombinedOutput() = %v\noutput: %s", err, out)
	}
	if err := os.Chdir("./perftest"); err != nil {
		t.Fatalf("os.Chdir(./perftest) = %v, want nil", err)
	}
	out, err = exec.CommandContext(ctx, "./autogen.sh").CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, ./autogen.sh).CombinedOutput() = %v, want nil\noutput: %s", err, out)
	}
	configure := exec.CommandContext(ctx, "./configure")
	configure.Env = append(configure.Environ(), "CUDA_H_PATH=/usr/local/cuda/include/cuda.h")
	out, err = configure.CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, CUDA_H_PATH=/usr/local/cuda/include/cuda.h ./configure).CombinedOutput() = %v, want nil\noutput: %s", err, out)
	}
	// -j$(nproc) causes compilation failures in some cases
	out, err = exec.CommandContext(ctx, "make").CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, make).CombinedOutput() = %v, want nil\noutput: %s", err, out)
	}
}

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
	setupPerftest(ctx, t)
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
	setupPerftest(ctx, t)
	ibWriteBWArgs := buildIBWriteBWArgs(ctx, t)
	target, err := utils.GetRealVMName(ctx, rdmaHostName)
	if err != nil {
		t.Fatalf("utils.GetRealVMName(%s) = %v, want nil", rdmaHostName, err)
	}
	ibWriteBWArgs = append(ibWriteBWArgs, target)
	for {
		ibWriteBWCmd := exec.CommandContext(ctx, "./ib_write_bw", ibWriteBWArgs...)
		out, err := ibWriteBWCmd.CombinedOutput()
		if err == nil {
			t.Logf("%s output:\n%s", ibWriteBWCmd.String(), out)
			break
		}
		// Client may be ready before host, retry connection errors.
		if strings.Contains(string(out), "Couldn't connect to "+target) {
			time.Sleep(time.Second)
			if ctx.Err() != nil {
				t.Logf("%s output:\n%s", ibWriteBWCmd.String(), out)
				t.Fatalf("context expired before connecting to host: %v\nlast ib_write_bw error was: %v", ctx.Err(), err)
			}
			continue
		}

		t.Logf("%s output:\n%s", ibWriteBWCmd.String(), out)
		t.Fatalf("exec.CommandContext(%s).CombinedOutput() = err %v, want nil", ibWriteBWCmd.String(), err)
	}
}
