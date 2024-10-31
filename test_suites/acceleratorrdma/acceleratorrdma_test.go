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
	"os"
	"os/exec"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Exercise the GPUDirectRDMA stack without involving NCCL by using
// https://github.com/linux-rdma/perftest.
// This is *not* a performance test, performance numbers aren't checked.
func TestA3UltraGPUDirectRDMA(t *testing.T) {
	ctx := utils.Context(t)
	var installBuildDepsCmd *exec.Cmd
	switch {
	case utils.CheckLinuxCmdExists("yum"):
		installBuildDepsCmd = exec.CommandContext(ctx, "yum", "install", "-y", "git", "cuda-toolkit", "perftest", "libtool", "automake", "autoconf", "make", "libibverbs-devel", "librdmacm-devel", "libibumad-devel", "pciutils-devel", "nvidia-driver-cuda-libs")
	case utils.CheckLinuxCmdExists("apt"):
		installBuildDepsCmd = exec.CommandContext(ctx, "apt", "install", "-y", "git", "ibverbs-utils", "cuda-toolkit", "perftest", "libtool", "automake", "autoconf", "libibverbs-dev", "librdmacm-dev", "libibumad-dev", "libpci-dev", "make")
	default:
		t.Fatalf("Unknown package manager, can't install build deps.")
	}
	if err := installBuildDepsCmd.Run(); err != nil {
		t.Fatalf("installBuildDepsCmd.Run() = %v, want nil", err)
	}
	if err := exec.CommandContext(ctx, "git", "clone", "--depth=1", "https://github.com/linux-rdma/perftest").Run(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, git, clone, --depth=1, https://github.com/linux-rdma/perftest).Run() = %v", err)
	}
	if err := os.Chdir("./perftest"); err != nil {
		t.Fatalf("os.Chdir(./perftest) = %v, want nil", err)
	}
	if err := exec.CommandContext(ctx, "./autogen.sh").Run(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, ./autogen.sh).Run() = %v, want nil", err)
	}
	configure := exec.CommandContext(ctx, "./configure")
	configure.Env = append(configure.Environ(), "CUDA_H_PATH=/usr/local/cuda/include/cuda.h")
	if err := configure.Run(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, CUDA_H_PATH=/usr/local/cuda/include/cuda.h ./configure).Run() = %v, want nil", err)
	}
	// -j$(nproc) causes compilation failures in some cases
	if err := exec.CommandContext(ctx, "make").Run(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, make).Run() = %v, want nil", err)
	}
	serverCmd := exec.CommandContext(ctx, "./ib_write_bw", "--report_gbits", "--use_cuda=0", "--use_cuda_dmabuf")
	if err := serverCmd.Start(); err != nil {
		t.Fatalf("%s.Start() = %v, want nil", serverCmd.String(), err)
	}
	clientCmd := exec.CommandContext(ctx, "./ib_write_bw", "--report_gbits", "--use_cuda=0", "--use_cuda_dmabuf", "localhost")
	if err := clientCmd.Start(); err != nil {
		t.Fatalf("%s.Start() = %v, want nil", clientCmd.String(), err)
	}

	if err := serverCmd.Wait(); err != nil {
		t.Fatalf("%s.Wait() = %v, want nil", serverCmd.String(), err)
	}
	if err := clientCmd.Wait(); err != nil {
		t.Fatalf("%s.Wait() = %v, want nil", clientCmd.String(), err)
	}
}
