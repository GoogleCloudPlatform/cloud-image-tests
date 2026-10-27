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
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func runOrFail(t *testing.T, cmd, msg string) {
	t.Helper()
	out, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		t.Fatalf("%s: stdout=%q stderr=%q err=%v", msg, out.Stdout, out.Stderr, err)
	}
}

func TestWaitForWinrmConnection(t *testing.T) {
	ctx := utils.Context(t)
	passwd, err := utils.GetMetadata(ctx, "instance", "attributes", "winrm-passwd")
	if err != nil {
		t.Fatalf("could not fetch winrm password: %v", err)
	}
	passwd = strings.TrimSpace(passwd)
	runOrFail(t, fmt.Sprintf("New-LocalUser -Name '%s' -Password (ConvertTo-SecureString -String '%s' -AsPlainText -Force)", user, passwd), fmt.Sprintf("could not add user %s", user))
	runOrFail(t, fmt.Sprintf("Add-LocalGroupMember -Group Administrators -Member %q", user), fmt.Sprintf("could not add user %s to administrators", user))
	t.Logf("winrm target boot succesfully at %d", time.Now().UnixNano())
}

func TestWinrmConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(utils.Context(t), 15*time.Minute)
	defer cancel()
	target, err := utils.GetRealVMName(ctx, "server")
	if err != nil {
		t.Fatalf("could not get target name: %v", err)
	}
	passwd, err := utils.GetMetadata(ctx, "instance", "attributes", "winrm-passwd")
	if err != nil {
		t.Fatalf("could not fetch winrm password: %v", err)
	}
	passwd = strings.TrimSpace(passwd)
	runOrFail(t, fmt.Sprintf("winrm set winrm/config/client '@{TrustedHosts=%q}'", target), "could not trust target")
	var lastOut utils.ProcessStatus
	for {
		if ctx.Err() != nil {
			t.Fatalf("test context expired before winrm was available: %v (last stdout: %q, stderr: %q)", ctx.Err(), lastOut.Stdout, lastOut.Stderr)
		}
		lastOut, err = utils.RunPowershellCmd(fmt.Sprintf("Test-WSMan %q", target))
		if err == nil {
			break
		}
		t.Logf("Test-WSMan %q failed (retrying): stdout=%q stderr=%q err=%v", target, lastOut.Stdout, lastOut.Stderr, err)
		time.Sleep(10 * time.Second)
	}
	for {
		if err := ctx.Err(); err != nil {
			t.Fatalf("test context expired before successful winrm connection: %v (last stdout: %q, stderr: %q)", err, lastOut.Stdout, lastOut.Stderr)
		}
		lastOut, err = utils.RunPowershellCmd(fmt.Sprintf(`Invoke-Command -SessionOption(New-PSSessionOption -SkipCACheck -SkipCNCheck -SkipRevocationCheck) -ScriptBlock{ hostname } -ComputerName %s -Credential (New-Object -TypeName System.Management.Automation.PSCredential -ArgumentList "%s\%s", (ConvertTo-SecureString -String '%s' -AsPlainText -Force))`, target, target, user, passwd))
		if err == nil && strings.Contains(lastOut.Stdout, "server-winrm") {
			break
		}
		t.Logf("Invoke-Command to %q failed (retrying): stdout=%q stderr=%q err=%v", target, lastOut.Stdout, lastOut.Stderr, err)
		time.Sleep(10 * time.Second)
	}
}
