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

package winrm

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func runOrFail(t *testing.T, cmd, msg string) {
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		t.Fatalf("%s: %s %s %v", msg, out.Stdout, out.Stderr, msg)
	}
}

func TestWaitForWinrmConnection(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	passwd, err := utils.GetMetadata(ctx, "instance", "attributes", "winrm-passwd")
	if err != nil {
		t.Fatalf("could not fetch winrm password: %v", err)
	}
	passwd = strings.TrimSpace(passwd)
	runOrFail(t, fmt.Sprintf(`net user "%s" "%s" /add`, user, passwd), fmt.Sprintf("could not add user %s", user))
	runOrFail(t, fmt.Sprintf(`Add-LocalGroupMember -Group Administrators -Member "%s"`, user), fmt.Sprintf("could not add user %s to administrators", user))
	t.Logf("winrm target boot succesfully at %d", time.Now().UnixNano())
}

func TestWinrmConnection(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	target, err := utils.GetRealVMName("server")
	if err != nil {
		t.Fatalf("could not get target name: %v", err)
	}
	passwd, err := utils.GetMetadata(ctx, "instance", "attributes", "winrm-passwd")
	if err != nil {
		t.Fatalf("could not fetch winrm password: %v", err)
	}
	passwd = strings.TrimSpace(passwd)
	runOrFail(t, fmt.Sprintf(`winrm set winrm/config/client '@{TrustedHosts="%s"}'`, target), "could not trust target")
	for {
		if ctx.Err() != nil {
			t.Fatalf("test context expired before winrm was available: %v", ctx.Err())
		}
		_, err := utils.RunPowershellCmd(fmt.Sprintf(`Test-WSMan "%s"`, target))
		time.Sleep(time.Minute) // Sleep even on success as there is some delay between target starting winrm and creating the test user
		if err == nil {
			break
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			t.Fatalf("test context expired before successful winrm connection: %v", err)
		}
		out, err := utils.RunPowershellCmd(fmt.Sprintf(`Invoke-Command -SessionOption(New-PSSessionOption -SkipCACheck -SkipCNCheck -SkipRevocationCheck) -ScriptBlock{ hostname } -ComputerName %s -Credential (New-Object -TypeName System.Management.Automation.PSCredential -ArgumentList "%s\%s", (ConvertTo-SecureString -String '%s' -AsPlainText -Force))`, target, target, user, passwd))
		if err == nil && strings.Contains(out.Stdout, "server-winrm") {
			break
		}
		time.Sleep(time.Minute)
	}
}
