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
	wickedPath = "/etc/sysconfig/network/ifcfg-%s"
)

// Verify wicked configuration.
//
// Because the guest agent does not override the wicked configuration if it
// exists, we don't verify the contents of the file.
func testWicked(t *testing.T, nic EthernetInterface, _ bool) {
	t.Helper()

	file := fmt.Sprintf(wickedPath, nic.Name)

	// Guest agent doesn't override the wicked configuration. So it will always
	// exist. Simply verify that the file exists.
	verifyFileExists(t, file, true)
}
