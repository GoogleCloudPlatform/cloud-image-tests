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

package acceleratorconfig

import (
	_ "embed"
	"os/exec"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/google/go-cmp/cmp"
)

//go:embed testdata/nvidia-smi-topo.txt
var wantTopo string

func TestNvidiaSmi(t *testing.T) {
	ctx := utils.Context(t)
	gotTopo, err := exec.CommandContext(ctx, "nvidia-smi", "topo", "-m").CombinedOutput()
	if err != nil {
		t.Fatalf("exec.CommandContext(ctx, nvidia-smi topo -m).CombinedOutput() failed unexpectedly; err = %v\noutput: %s", err, gotTopo)
	}
	t.Logf("nvidia-smi topo -m output:\n%s", gotTopo)

	if diff := cmp.Diff(gotTopo, wantTopo); diff != "" {
		t.Errorf("nvidia-smi topo -m returned unexpected diff (-want +got):\n%s", diff)
	}
}
