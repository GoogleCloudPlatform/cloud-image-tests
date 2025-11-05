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

package acceleratornccl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/acceleratorutils"
)

// findMPIBinary finds the path to the specified MPI command binary.
func findMPIBinary(ctx context.Context, t *testing.T, command string) string {
	t.Helper()
	path, err := exec.LookPath(command)
	if err == nil {
		t.Logf("Found %s in PATH: %s", command, path)
		return path
	}
	// In Rocky Linux openmpi is installed in versioned directories.
	mpiGlobPath := "/usr/mpi/gcc/openmpi-*/bin/" + command
	t.Logf("Could not find mpi binary %q in PATH. Searching in %q", command, mpiGlobPath)
	matches, err := filepath.Glob(mpiGlobPath)
	if err == nil && len(matches) > 0 {
		t.Logf("Found command %q: %q", command, matches[len(matches)-1])
		return matches[len(matches)-1]
	}
	t.Fatalf("Could not find mpi binary %q in PATH or %q: %v", command, mpiGlobPath, err)
	return ""
}

// findMPIHome finds the MPI_HOME directory.
func findMPIHome(ctx context.Context, t *testing.T) (mpiHome string) {
	t.Helper()
	// mpicc --showme returns the command used to compile mpicc which includes the MPI_HOME path.
	// Example: gcc -I/usr/lib/x86_64-linux-gnu/openmpi/include ...
	mpicc := findMPIBinary(ctx, t, "mpicc")
	showMeCmd := exec.CommandContext(ctx, mpicc, "--showme")
	out, err := showMeCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Command %v failed. Cannot determine MPI_HOME paths. Error: %v", showMeCmd, err)
	}
	fields := strings.Fields(string(out))
	for _, field := range fields {
		// MPI_HOME is the parent directory of the first "-I" include path.
		if strings.HasPrefix(field, "-I") {
			includePath := strings.TrimPrefix(field, "-I")
			mpiHome = filepath.Dir(includePath)
			mpiHome = fmt.Sprintf("MPI_HOME=%s", mpiHome)
			t.Logf("Found mpi home: %q", mpiHome)
			return mpiHome
		}
	}
	t.Fatalf("Could not find MPI_HOME using mpicc --showme")
	return ""
}

func setupNCCLTest(ctx context.Context, t *testing.T) {
	t.Helper()
	acceleratorutils.InstallCudaRuntime(ctx, t)
	switch {
	case utils.CheckLinuxCmdExists("yum"):
		if err := utils.InstallPackage("git", "gcc", "gcc-c++", "make"); err != nil {
			t.Fatalf("Failed to install git, gcc, gcc-c++, and make: %v", err)
		}
	case utils.CheckLinuxCmdExists("apt"):
		if err := utils.InstallPackage("git", "build-essential", "libopenmpi-dev"); err != nil {
			t.Fatalf("Failed to install git, build-essential, and libopenmpi-dev: %v", err)
		}
	default:
		t.Fatalf("Unknown package manager (not yum or apt), can't install build deps.")
	}

	// Install and build nccl
	out, err := exec.CommandContext(ctx, "git", "clone", "--depth=1", "https://github.com/NVIDIA/nccl.git").CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, git, clone, --depth=1, https://github.com/NVIDIA/nccl.git).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", err, out)
	}
	if err := os.Chdir("nccl"); err != nil {
		t.Fatalf("os.Chdir(nccl) = %v, want nil", err)
	}
	if out, err := exec.CommandContext(ctx, "make", "-j", "src.build").CombinedOutput(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, make, -j, src.build).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", err, out)
	}
	ncclBuildPath, err := filepath.Abs("build")
	t.Logf("nccl build directory: %s", ncclBuildPath)
	if err != nil {
		t.Fatalf("filepath.Abs(build) failed unexpectedly; err = %v", err)
	}
	// Add nccl/build/lib directory to LD_LIBRARY_PATH which is needed to invoke mpirun later.
	ldLibraryPath := fmt.Sprintf("%s/lib:%s", ncclBuildPath, os.Getenv("LD_LIBRARY_PATH"))
	if err := os.Setenv("LD_LIBRARY_PATH", ldLibraryPath); err != nil {
		t.Fatalf("os.Setenv(LD_LIBRARY_PATH, %s) failed unexpectedly; err = %v", ldLibraryPath, err)
	}
	if err := os.Chdir(".."); err != nil {
		t.Fatalf("os.Chdir(..) = %v, want nil", err)
	}

	// Install and build nccl-tests
	if out, err := exec.CommandContext(ctx, "git", "clone", "--depth=1", "https://github.com/NVIDIA/nccl-tests.git").CombinedOutput(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, git, clone, --depth=1, https://github.com/NVIDIA/nccl-tests.git).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", err, out)
	}
	if err := os.Chdir("nccl-tests"); err != nil {
		t.Fatalf("os.Chdir(nccl-tests) = %v, want nil", err)
	}
	makeCmd := exec.CommandContext(ctx, "make", "-j8", "MPI=1")
	mpiHome := findMPIHome(ctx, t)
	ncclHome := fmt.Sprintf("NCCL_HOME=%s", ncclBuildPath)
	makeCmd.Env = append(os.Environ(), mpiHome, ncclHome)
	if out, err := makeCmd.CombinedOutput(); err != nil {
		t.Fatalf("Command %v failed unexpectedly; err = %v\noutput: %s", makeCmd, err, out)
	}
}

var ncclTests = []string{
	"alltoall_perf",
	"all_gather_perf",
	"all_reduce_perf",
}

var ncclTestArgs = []string{
	"--minbytes", "8",
	"--maxbytes", "8G",
	"--stepfactor", "2",
	"--ngpus", "1", // number of GPUs per process
}

var mpiArgs = []string{
	"-np", "8", // Number of processes
	"-N", "8", // Number of processes per node
}

func TestNCCL(t *testing.T) {
	ctx := utils.Context(t)
	setupNCCLTest(ctx, t)
	mpirun := findMPIBinary(ctx, t, "mpirun")
	for _, ncclTest := range ncclTests {
		args := append(mpiArgs, "./build/"+ncclTest)
		args = append(args, ncclTestArgs...)
		ncclTestCmd := exec.CommandContext(ctx, mpirun, args...)
		ncclTestCmd.Env = append(os.Environ(), "OMPI_ALLOW_RUN_AS_ROOT=1", "OMPI_ALLOW_RUN_AS_ROOT_CONFIRM=1")
		out, err := ncclTestCmd.CombinedOutput()
		t.Logf("NCCL test output, command: %s:\n%s", ncclTestCmd, out)
		if err != nil {
			t.Errorf("NCCL test failed unexpectedly; command: %s, err = %v\noutput: %s", ncclTestCmd, err, out)
		}
	}
}
