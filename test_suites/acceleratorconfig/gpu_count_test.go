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
	"encoding/xml"
	"os/exec"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

type Data struct {
	Gpucount string `xml:"attached_gpus"`
}

func TestA3UGpuCount(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	var data Data
	var wantedCount = "8"
	cmd := exec.CommandContext(ctx, "nvidia-smi", "-x", "-q")
	if res, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exec.CommandContext(ctx, \"nvidia-smi\", \"-x\", \"-q\"): %v, want nil", err)
	} else {
		t.Logf("nvidia-smi output: %s", string(res))
		if err := xml.Unmarshal([]byte(res), &data); err != nil {
			t.Fatalf("xml.Unmarshal: %v, want nil ", err)
		}
	}

	if data.Gpucount != wantedCount {
		t.Fatalf("TestA3UGpuCount: Got %s, want %s", data.Gpucount, wantedCount)
	}
}
