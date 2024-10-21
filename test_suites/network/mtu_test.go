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

package network

import (
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	gceMTU = 1460
)

func TestDefaultMTU(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	iface, err := utils.GetInterface(ctx, 0)
	if err != nil {
		t.Fatalf("couldn't find primary NIC: %v", err)
	}
	if iface.MTU != gceMTU {
		t.Fatalf("expected MTU %d on interface %s, got MTU %d", gceMTU, iface.Name, iface.MTU)
	}
}
