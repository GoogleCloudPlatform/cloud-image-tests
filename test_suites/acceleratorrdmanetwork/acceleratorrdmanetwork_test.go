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

package acceleratorrdmanetwork

import (
	"os/exec"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils/acceleratorutils"
)

var pingPongArgs = []string{
	"--gid-idx=3",
}

func TestRDMANetworkHost(t *testing.T) {
	ctx := utils.Context(t)
	acceleratorutils.InstallIbVerbsUtils(ctx, t)
	pingPongCmd := exec.CommandContext(ctx, "ibv_rc_pingpong", pingPongArgs...)
	out, err := pingPongCmd.CombinedOutput()
	t.Logf("%s output:\n%s", pingPongCmd, out)
	if err != nil {
		t.Fatalf("exec.CommandContext(%q).CombinedOutput() failed unexpectedly; err = %v", pingPongCmd, err)
	}
}

func TestRDMANetworkClient(t *testing.T) {
	ctx := utils.Context(t)
	acceleratorutils.InstallIbVerbsUtils(ctx, t)
	acceleratorutils.RunRDMAClientCommand(ctx, t, "ibv_rc_pingpong", pingPongArgs)
}
