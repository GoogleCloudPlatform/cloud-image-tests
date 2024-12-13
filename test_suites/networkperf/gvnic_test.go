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

package networkperf

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

const (
	driverPath = "/sys/class/net/%s/device/driver"
)

func CheckGVNICPresent(interfaceName string) error {
	file := fmt.Sprintf(driverPath, interfaceName)
	data, err := os.Readlink(file)
	if err != nil {
		return err
	}
	s := strings.Split(data, "/")
	driver := s[len(s)-1]
	if driver != "gvnic" && driver != "gve" {
		errMsg := fmt.Sprintf("Driver set as %v want gvnic or gve", driver)
		return errors.New(errMsg)
	}
	return nil
}

func CheckGVNICPresentWindows(interfaceName string) error {
	command := fmt.Sprintf("Get-NetAdapter -Name \"%s\"", interfaceName)
	result, err := utils.RunPowershellCmd(command)
	if err != nil {
		return err
	}
	if strings.Contains(result.Stdout, "Google Ethernet Adapter") {
		return nil
	}
	return errors.New("GVNIC not present")
}

func TestGVNICExists(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	iface, err := utils.GetInterface(ctx, 0)
	if err != nil {
		t.Fatalf("couldn't find primary NIC: %v", err)
	}
	var errMsg error
	if runtime.GOOS == "windows" {
		errMsg = CheckGVNICPresentWindows(iface.Name)
	} else {
		errMsg = CheckGVNICPresent(iface.Name)
	}
	if errMsg != nil {
		t.Fatalf("Error: %v", errMsg.Error())
	}
}
