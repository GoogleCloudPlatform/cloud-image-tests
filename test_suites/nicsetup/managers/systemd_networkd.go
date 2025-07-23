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

package managers

import (
	"fmt"
	"testing"
)

const (
	systemdNetworkdPath = "/usr/lib/systemd/network/20-%s-google-guest-agent.network"
)

// TODO(b/431298674): Verify systemd-networkd configuration.
func testSystemdNetworkd(t *testing.T, nic EthernetInterface, exist bool) {
	t.Helper()

	file := fmt.Sprintf(systemdNetworkdPath, nic.Name)
	verifyFileExists(t, file, exist)
}
