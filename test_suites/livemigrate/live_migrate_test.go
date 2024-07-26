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

package livemigrate

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func TestLiveMigrate(t *testing.T) {
	marker := "/var/lm-test-start"
	if utils.IsWindows() {
		marker = `C:\lm-test-start`
	}
	if _, err := os.Stat(marker); err != nil && !os.IsNotExist(err) {
		t.Fatalf("could not determine if live migrate testing has already started: %v", err)
	} else if err == nil {
		t.Fatal("unexpected reboot during live migrate test")
	}
	err := os.WriteFile(marker, nil, 0777)
	if err != nil {
		t.Fatalf("could not mark beginning of live migrate testing: %v", err)
	}
	ctx := utils.Context(t)
	prj, zone, err := utils.GetProjectZone(ctx)
	if err != nil {
		t.Fatalf("could not find project and zone: %v", err)
	}
	inst, err := utils.GetInstanceName(ctx)
	if err != nil {
		t.Fatalf("could not get instance: %v", err)
	}
	client, err := utils.GetDaisyClient(ctx)
	if err != nil {
		t.Fatalf("could not make compute api client: %v", err)
	}

	err = client.SimulateMaintenanceEvent(prj, zone, inst)
	if err != nil {
		// Operation errors here come from things completely out of our control, such as the availability of a physical machine to take our VM.
		if !strings.Contains(err.Error(), "MIGRATION_TEMPORARILY_UNAVAILABLE") {
			t.Fatalf("could not migrate self: %v", err)
		}
	}

	if _, err := os.Stat(marker); err != nil {
		t.Errorf("could not confirm migrate testing has started ok: %v", err)
	}
	_, err = http.Get("https://cloud.google.com/")
	if err != nil {
		t.Errorf("lost network connection after live migration")
	}
}
