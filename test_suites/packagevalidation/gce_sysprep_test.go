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

package packagevalidation

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func resetDebugLogs(t *testing.T) {
	t.Helper()
	script := `
(Get-Content -Path "C:\Program Files\Google\Compute Engine\instance_configs.cfg") -replace 'log_level = 4', 'log_level = 3' | Set-Content -Path "C:\Program Files\Google\Compute Engine\instance_configs.cfg"
(Get-Content -Path "C:\Program Files\Google\Compute Engine\instance_configs.cfg") -replace 'log_verbosity = 4', 'log_verbosity = 0' | Set-Content -Path "C:\Program Files\Google\Compute Engine\instance_configs.cfg"
`

	out, err := utils.RunPowershellCmd(script)
	if err != nil {
		t.Fatalf("Failed to reset log settings: %v, output: %+v", err, out)
	}
}

func TestGCESysprep(t *testing.T) {
	utils.WindowsOnly(t)
	resetDebugLogs(t)
	ctx := utils.Context(t)
	err := os.WriteFile(`C:\Windows\Temp\test.txt`, []byte(`test file`), 0777)
	if err != nil {
		t.Fatal(err)
	}

	out, err := exec.CommandContext(ctx, "GCESysprep", "-NoShutdown").CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run gcesysprep: %s %v", out, err)
	}

	// RecordCount under 200 is acceptable, some logs will be generated after clearing the log during the rest of the sysprep
	logs, err := utils.RunPowershellCmd(`Get-WinEvent -ListLog * | Where-Object {$_.RecordCount -gt 200} | Format-Table -HideTableHeaders -Property LogName,RecordCount`)
	if err != nil {
		t.Fatalf("could not get eventlog counts: %s %v", logs.Stderr, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(logs.Stdout), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line != "" {
			t.Errorf("found log with too many entries: %q", line)
		}
	}

	_, err = os.Stat(`C:\Windows\Temp\test.txt`)
	if err == nil {
		t.Error(`C:\Windows\Temp was not cleared`)
	}
	if !os.IsNotExist(err) {
		t.Errorf(`error checking for existence of C:\Windows\Temp\test.txt: %v`, err)
	}
	winrmCert, err := utils.RunPowershellCmd(`(Get-ChildItem 'Cert:\LocalMachine\My\').Thumbprint`)
	if err != nil {
		t.Fatalf("failed to get winrm cert thumbprint: %s %v", winrmCert.Stderr, err)
	}
	if strings.TrimSpace(winrmCert.Stdout) != "" {
		t.Errorf("winrm cert thumbprint is not empty\npowershell command (Get-ChildItem 'Cert:\\LocalMachine\\My\\').Thumbprint = %q, want \"\"", strings.TrimSpace(winrmCert.Stdout))
	}
	rpdCert, err := utils.RunPowershellCmd(`(Get-ChildItem 'Cert:\LocalMachine\Remote Desktop\').Thumbprint`)
	if err != nil {
		t.Fatalf("failed to get rpd cert thumbprint: %s %v", rpdCert.Stderr, err)
	}
	if strings.TrimSpace(rpdCert.Stdout) != "" {
		t.Errorf("rpd cert thumbprint is not empty\npowershell command (Get-ChildItem 'Cert:\\LocalMachine\\Remote Desktop\\').Thumbprint = %q, want \"\"", strings.TrimSpace(rpdCert.Stdout))
	}
	disks, err := utils.RunPowershellCmd(`Get-ChildItem 'HKLM:\SYSTEM\CurrentControlSet\Enum\SCSI\Disk&Ven_Google&Prod_PersistentDisk\*\Device Parameters\Partmgr'`)
	if err == nil && strings.TrimSpace(disks.Stdout) != "" {
		t.Errorf("known disk configs were not cleared in gcesysprep, found %q", strings.TrimSpace(disks.Stdout))
	}
	tasks, err := exec.CommandContext(ctx, "schtasks", "/query", "/tn", "GCEStartup", "/nh", "/fo", "csv").Output()
	if err != nil {
		t.Fatalf("could not get scheduled tasks: %v", err)
	}
	for _, line := range strings.Split(string(tasks), "\n") {
		line = strings.TrimSuffix(line, "\r")
		fields := strings.Split(line, ",")
		if len(fields) < 3 || fields[0] != `"\GCEStartup"` {
			continue
		}
		if fields[2] != `"Disabled"` {
			t.Errorf(`unexpected GCEStartup task status, want "Disabled" but got %s`, fields[2])
		}
	}
	setupCompleteLoc, _ := os.LookupEnv("WinDir")
	setupCompleteLoc += `\Setup\Scripts\SetupComplete.cmd`
	_, err = os.Stat(setupCompleteLoc)
	if os.IsNotExist(err) {
		t.Errorf(`Could not find SetupComplete.cmd at %s: %v`, setupCompleteLoc, err)
	}
	winrmFw, err := utils.RunPowershellCmd(`Get-NetFirewallRule -DisplayName 'Windows Remote Management (HTTPS-In)' | Where-Object {$_.Direction -eq "Inbound" -and $_.Profile -eq "Any" -and $_.Action -eq "Allow" -and $_.Enabled -eq $True } | Get-NetFirewallPortFilter | Where-Object {$_.Protocol -eq "TCP" -and $_.LocalPort -eq 5986 }`)
	if err != nil {
		t.Fatalf("could not check for existence of winrm firewall rule: %v", err)
	}
	if strings.TrimSpace(winrmFw.Stdout) == "" {
		t.Errorf("could not find winrm firewall rule")
	}
	rdpFw, err := utils.RunPowershellCmd(`Get-NetFirewallRule -DisplayGroup 'Remote Desktop' | Where-Object {$_.Enabled -eq $True }`)
	if err != nil {
		t.Fatalf("could not check for existence of rdp firewall rule: %v", err)
	}
	if strings.TrimSpace(rdpFw.Stdout) == "" {
		t.Errorf("could not find rdp firewall rule")
	}
	sysprepInstalled, err := utils.RunPowershellCmd(`googet installed google-compute-engine-sysprep.noarch`)
	if err != nil {
		t.Fatalf("could not check installed sysprep version: %v, stdoutput: %s stderr: %s", err, sysprepInstalled.Stdout, sysprepInstalled.Stderr)
	}
	// YYYYMMDD
	sysprepVerRe := regexp.MustCompile("[0-9]{8}")
	sysprepVer, err := strconv.Atoi(sysprepVerRe.FindString(sysprepInstalled.Stdout))
	if err != nil {
		t.Fatalf("could not determine value of sysprep version: %v", err)
	}
	if sysprepVer <= 20240122 {
		t.Skipf("version %d of gcesysprep is too old to disable google_osconfig_agent when -NoShutdown is passed", sysprepVer)
	}
	osconfigAgentStatus, err := utils.RunPowershellCmd(`Get-Service google_osconfig_agent | Where-Object {$_.StartType -eq "Disabled"}`)
	if err != nil || osconfigAgentStatus.Stdout == "" {
		t.Errorf("google_osconfig_agent does not appear to be disabled: %s %v", osconfigAgentStatus.Stdout, err)
	}
}
