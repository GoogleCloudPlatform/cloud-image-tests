// Copyright 2025 Google LLC.
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

package nicsetup

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/nicsetup/managers"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	// instanceConfigLoc is the location of the instance config file to modify.
	// .template takes priority over the non-template files. This file shouldn't
	// exist by default on any image.
	instanceConfigLoc = "/etc/default/instance_configs.cfg.template"
)

// getNumInterfaces returns the number of interfaces set by the setup.
func getNumInterfaces(t *testing.T) int {
	t.Helper()
	val, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "network-interfaces-count")
	if err != nil {
		t.Fatalf("couldn't get network-interfaces-count from metadata: %v", err)
	}
	num, err := strconv.Atoi(val)
	if err != nil {
		t.Fatalf("couldn't convert network-interfaces-count to int: %v", err)
	}
	return num
}

// enablePrimaryNIC enables/disables the primary NIC configuration.
func enablePrimaryNIC(t *testing.T, enable bool) {
	t.Helper()
	enablePrimaryNICConfig := fmt.Sprintf(`[NetworkInterfaces]
manage_primary_nic = %t
`, enable)

	// Write the instance config file.
	if err := os.WriteFile(instanceConfigLoc, []byte(enablePrimaryNICConfig), 0644); err != nil {
		t.Fatalf("couldn't write to instance config file: %v", err)
	}

	// Restart the guest agent.
	if err := utils.RestartAgent(utils.Context(t)); err != nil {
		t.Fatalf("couldn't restart guest agent: %v", err)
	}

	// Wait for the guest agent to restart.
	time.Sleep(time.Second * 10)
}

func TestPrimaryNIC(t *testing.T) {
	nic := managers.GetNIC(t, 0)

	// Check that no configurations for the primary NIC exist.
	managers.VerifyNIC(t, nic, false)

	// Enable primary NIC configuration.
	enablePrimaryNIC(t, true)
	t.Logf("Enabled primary NIC configuration")

	// Check the configurations for the primary NIC exist.
	managers.VerifyNIC(t, nic, true)

	// Disable primary NIC configuration.
	enablePrimaryNIC(t, false)
	t.Logf("Disabled primary NIC configuration")

	// Check that the configurations for the primary NIC don't exist.
	managers.VerifyNIC(t, nic, false)
}

func TestSecondaryNIC(t *testing.T) {
	numInterfaces := getNumInterfaces(t)
	if numInterfaces != 2 {
		t.Skipf("Secondary NIC test only runs on multi-NIC VMs")
	}

	nic := managers.GetNIC(t, 1)

	// Check the configurations for the secondary NIC exist.
	managers.VerifyNIC(t, nic, true)
}
