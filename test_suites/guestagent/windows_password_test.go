//  Copyright 2018 Google LLC.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package guestagent

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	// allowlist:crypto/sha1
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	daisyCompute "github.com/GoogleCloudPlatform/compute-daisy/compute"
)

const (
	// passwordResetUser is the user for the password reset test.
	passwordResetUser = "windowsuser5"
	// initialTestUserPassword is the initial local password set before testing password reset.
	initialTestUserPassword = "gyug3q445m0!"
	// differentLocaleUser is the user for the password reset test with a different locale.
	// This is currently just the Romaji spelling of "Administrators" in Japanese, with
	// "user" appended to the end.
	differentLocaleUser = "kanrininuser"
	// This is the well-known SID for the local admin group.
	// https://learn.microsoft.com/en-us/windows-server/identity/ad-ds/manage/understand-security-identifiers#well-known-sids
	localAdminSID = "S-1-5-32-544"
	// This is the name of the admin group that the normal Administrators group is renamed to.
	// This is currently just the romaji spelling of "Administrators" in Japanese.
	newAdminGroupName       = "kanrinin"
	windowsKeysMetadataKey  = "windows-keys"
	credCheckUserEnvVar     = "CIT_CRED_USER"
	credCheckPasswordEnvVar = "CIT_CRED_PASSWORD"
	rsaKeyBitSize           = 2048
	com4SerialPort          = 4
	windowsKeyTTL           = 10 * time.Minute
	com4ResponseTimeout     = 2 * time.Minute
	agentPollInterval       = 3 * time.Second
)

type windowsKeyJSON struct {
	ExpireOn string
	Exponent string
	Modulus  string
	UserName string
}

type credsJSON struct {
	ErrorMessage      string `json:"errorMessage,omitempty"`
	EncryptedPassword string `json:"encryptedPassword,omitempty"`
	UserName          string `json:"userName,omitempty"`
	PasswordFound     bool   `json:"passwordFound,omitempty"`
	Modulus           string `json:"modulus,omitempty"`
}

func psSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func decryptPassword(priv *rsa.PrivateKey, ep string) (string, error) {
	bp, err := base64.StdEncoding.DecodeString(ep)
	if err != nil {
		return "", fmt.Errorf("error decoding password: %v", err)
	}
	pwd, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, priv, bp, nil)
	if err != nil {
		return "", fmt.Errorf("error decrypting password: %v", err)
	}
	return string(pwd), nil
}

func currentSerialPortOffset(client daisyCompute.Client, projectID, zone, instanceName string, port int64) (int64, error) {
	out, err := client.GetSerialPortOutput(projectID, zone, instanceName, port, 0)
	if err != nil {
		return 0, fmt.Errorf("failed to query serial port %d offset for %s: %w", port, instanceName, err)
	}
	if out == nil {
		return 0, fmt.Errorf("nil serial port %d output for %s", port, instanceName)
	}
	return out.Next, nil
}

func requestWindowsPasswordReset(ctx context.Context, client daisyCompute.Client, user string) (*credsJSON, *rsa.PrivateKey, error) {
	instanceName, err := utils.GetInstanceName(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get instance name: %w", err)
	}
	projectID, zone, err := utils.GetProjectZone(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get project or zone: %w", err)
	}

	key, err := rsa.GenerateKey(rand.Reader, rsaKeyBitSize)
	if err != nil {
		return nil, nil, err
	}
	bs := binary.BigEndian.AppendUint32(nil, uint32(key.PublicKey.E))
	winKey := &windowsKeyJSON{
		ExpireOn: time.Now().UTC().Add(windowsKeyTTL).Format(time.RFC3339),
		// This is different from what other tools produce (AQAB vs AQABAA==),
		// though both decode to 65537.
		Exponent: base64.StdEncoding.EncodeToString(bs),
		Modulus:  base64.StdEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		UserName: user,
	}
	data, err := json.Marshal(winKey)
	if err != nil {
		return nil, nil, err
	}

	offset, err := currentSerialPortOffset(client, projectID, zone, instanceName, com4SerialPort)
	if err != nil {
		return nil, nil, err
	}
	if err := utils.UpsertMetadata(ctx, windowsKeysMetadataKey, string(data)); err != nil {
		return nil, nil, fmt.Errorf("failed to upsert %q metadata for %q: %w", windowsKeysMetadataKey, user, err)
	}

	var matched *credsJSON
	err = utils.PollUntil(ctx, fmt.Sprintf("COM4 password reset response for %q", user), agentPollInterval, com4ResponseTimeout, func() (bool, string) {
		out, rpcErr := client.GetSerialPortOutput(projectID, zone, instanceName, com4SerialPort, offset)
		if rpcErr == nil && out != nil && out.Contents == "" && out.Next < offset {
			offset = 0
			out, rpcErr = client.GetSerialPortOutput(projectID, zone, instanceName, com4SerialPort, 0)
		}
		if rpcErr != nil {
			return false, fmt.Sprintf("modulus=%s, last RPC err=%v", winKey.Modulus, rpcErr)
		}
		if out == nil {
			return false, fmt.Sprintf("modulus=%s, nil serial port output", winKey.Modulus)
		}
		for _, line := range strings.Split(out.Contents, "\n") {
			var creds credsJSON
			if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &creds); err != nil {
				continue
			}
			if creds.Modulus == winKey.Modulus {
				matched = &creds
				return true, ""
			}
		}
		return false, fmt.Sprintf("modulus=%s, recent COM4 output=%q", winKey.Modulus, out.Contents)
	})
	if err != nil {
		return nil, nil, err
	}
	return matched, key, nil
}

func decryptResetCreds(t *testing.T, user string, creds *credsJSON, privKey *rsa.PrivateKey) string {
	t.Helper()
	if creds.ErrorMessage != "" {
		t.Fatalf("password reset for %q returned errorMessage = %q, want empty", user, creds.ErrorMessage)
	}
	if !creds.PasswordFound || creds.EncryptedPassword == "" {
		t.Fatalf("password reset for %q returned passwordFound=%t, encryptedPassword empty=%t, want true and false", user, creds.PasswordFound, creds.EncryptedPassword == "")
	}
	pwd, err := decryptPassword(privKey, creds.EncryptedPassword)
	if err != nil {
		t.Fatalf("failed to decrypt password for %q: %v", user, err)
	}
	return pwd
}

func redactSecret(s, secret string) string {
	trimmed := strings.TrimSpace(s)
	if secret == "" {
		return trimmed
	}
	return strings.ReplaceAll(trimmed, secret, "<redacted>")
}

func verifyLocalWindowsCredentials(ctx context.Context, t *testing.T, username, password string) {
	t.Helper()
	credEnv := []string{
		fmt.Sprintf("%s=%s", credCheckUserEnvVar, username),
		fmt.Sprintf("%s=%s", credCheckPasswordEnvVar, password),
	}
	validateCmd := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$user = [string]$env:%[1]s
$plainPwd = [string]$env:%[2]s
Remove-Item Env:\%[1]s, Env:\%[2]s -ErrorAction SilentlyContinue
Add-Type -AssemblyName System.DirectoryServices.AccountManagement
$ctx = New-Object System.DirectoryServices.AccountManagement.PrincipalContext([System.DirectoryServices.AccountManagement.ContextType]::Machine)
if (-not $ctx.ValidateCredentials($user, $plainPwd)) {
  throw 'ValidateCredentials returned false for local account'
}
`, credCheckUserEnvVar, credCheckPasswordEnvVar)
	if procStatus, err := utils.RunPowershellCmdWithEnv(ctx, validateCmd, credEnv...); err != nil || procStatus.Exitcode != 0 {
		t.Fatalf("ValidateCredentials for %q failed (exitcode=%d, err=%v): stderr=%s", username, procStatus.Exitcode, err, redactSecret(procStatus.Stderr, password))
	}
	logonCmd := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$user = [string]$env:%[1]s
$secPwd = ConvertTo-SecureString ([string]$env:%[2]s) -AsPlainText -Force
Remove-Item Env:\%[1]s, Env:\%[2]s -ErrorAction SilentlyContinue
$cred = New-Object System.Management.Automation.PSCredential($user, $secPwd)
Start-Process -Credential $cred -WorkingDirectory C:\Windows\System32 -FilePath cmd.exe -ArgumentList '/c exit 0' -Wait -ErrorAction Stop
`, credCheckUserEnvVar, credCheckPasswordEnvVar)
	if procStatus, err := utils.RunPowershellCmdWithEnv(ctx, logonCmd, credEnv...); err != nil || procStatus.Exitcode != 0 {
		t.Fatalf("Start-Process logon check for %q failed (exitcode=%d, err=%v): stderr=%s", username, procStatus.Exitcode, err, redactSecret(procStatus.Stderr, password))
	}
}

func cleanupLocalWindowsUsers(t *testing.T, users ...string) {
	t.Helper()
	for _, u := range users {
		cmd := fmt.Sprintf("if ($null -ne (Get-LocalUser -Name '%[1]s' -ErrorAction SilentlyContinue)) { Remove-LocalUser -Name '%[1]s' -ErrorAction Stop } else { exit 0 }", psSingleQuote(u))
		if procStatus, err := utils.RunPowershellCmd(cmd); err != nil || procStatus.Exitcode != 0 {
			t.Errorf("failed to remove local test user %q (exitcode=%d, err=%v): stderr=%s", u, procStatus.Exitcode, err, strings.TrimSpace(procStatus.Stderr))
		}
	}
}

// Verifies that a powershell command ran with no errors and exited with an exit code of 0.
// If a username or password was invalid, this should result in a testing error.
// Returns the standard output in case it needs to be used later.
func verifyPowershellCmd(t *testing.T, cmd string) string {
	procStatus, err := utils.RunPowershellCmd(cmd)
	if err != nil {
		t.Fatalf("cmd %s failed: stdout %s, stderr %v, error %v", cmd, procStatus.Stdout, procStatus.Stderr, err)
	}

	stdout := procStatus.Stdout
	if procStatus.Exitcode != 0 {
		t.Fatalf("cmd %s failed with exitcode %d, stdout %s and stderr %s", cmd, procStatus.Exitcode, stdout, procStatus.Stderr)
	}
	return stdout
}

// TestWindowsPasswordReset tests that the guest agent can reset the password of a user.
// This test creates a user, then sends a request via MDS to the guest agent to reset the password.
// Since the user already exists, the guest agent should just reset the password.
// There are no expectations for the user to be added to the admin group, if it's
// not already there.
func TestWindowsPasswordReset(t *testing.T) {
	utils.WindowsOnly(t)
	ctx := utils.Context(t)
	client, err := utils.GetDaisyClient(ctx)
	if err != nil {
		t.Fatalf("Error creating compute service: %v", err)
	}

	t.Cleanup(func() {
		// t.Context() is already canceled when t.Cleanup runs, and daisyCompute.Client
		// bounds GCE operation waits internally without per-call contexts.
		if err := utils.DeleteMetadata(context.Background(), windowsKeysMetadataKey); err != nil {
			t.Errorf("failed to delete %s metadata in cleanup: %v", windowsKeysMetadataKey, err)
		}
		cleanupLocalWindowsUsers(t, passwordResetUser)
	})

	createUserCmd := fmt.Sprintf("net user %s %s /add", passwordResetUser, initialTestUserPassword)
	verifyPowershellCmd(t, createUserCmd)

	t.Logf("Resetting password on current instance for user %q", passwordResetUser)
	creds, privKey, err := requestWindowsPasswordReset(ctx, client, passwordResetUser)
	if err != nil {
		t.Fatalf("reset password failed: %v", err)
	}
	decryptedPassword := decryptResetCreds(t, passwordResetUser, creds, privKey)
	verifyLocalWindowsCredentials(ctx, t, passwordResetUser, decryptedPassword)
}

// TestWindowsPasswordResetDifferentLocale tests that the guest agent can create
// a user and add it to the local admin group, where the local admin group is
// renamed. The renaming of the group simply mocks a different locale, but
// the guest agent should still be expected to be able to add the user to that
// group.
func TestWindowsPasswordResetDifferentLocale(t *testing.T) {
	utils.WindowsOnly(t)
	if !utils.AgentFeatureCheck(t) {
		t.Skip("Skipping test as it tests a feature that is not currently generally available in the guest agent.")
	}

	// This feature is only implemented in the core plugin.
	if utils.IsCoreDisabled() {
		t.Skip("Skipping test as core plugin is disabled")
	}
	ctx := utils.Context(t)
	client, err := utils.GetDaisyClient(ctx)
	if err != nil {
		t.Fatalf("Error creating compute service: %v", err)
	}

	t.Cleanup(func() {
		if err := utils.DeleteMetadata(context.Background(), windowsKeysMetadataKey); err != nil {
			t.Errorf("failed to delete %s metadata in cleanup: %v", windowsKeysMetadataKey, err)
		}
		cleanupLocalWindowsUsers(t, differentLocaleUser)
		if procStatus, err := utils.RunPowershellCmd(fmt.Sprintf("if ($null -ne (Get-LocalGroup -Name '%[1]s' -ErrorAction SilentlyContinue)) { Rename-LocalGroup -Name '%[1]s' -NewName Administrators -ErrorAction Stop } else { exit 0 }", psSingleQuote(newAdminGroupName))); err != nil || procStatus.Exitcode != 0 {
			t.Errorf("failed to restore Administrators group name (exitcode=%d, err=%v): stderr=%s", procStatus.Exitcode, err, strings.TrimSpace(procStatus.Stderr))
		}
	})

	// Change the name of the admin group to something else.
	changeAdminGroupCmd := fmt.Sprintf("Rename-LocalGroup -Name Administrators -NewName %s -ErrorAction Stop", newAdminGroupName)
	verifyPowershellCmd(t, changeAdminGroupCmd)

	// Reset the password for the user.
	t.Logf("Resetting password on current instance for user %q", differentLocaleUser)
	creds, privKey, err := requestWindowsPasswordReset(ctx, client, differentLocaleUser)
	if err != nil {
		t.Fatalf("reset password failed: %v", err)
	}
	decryptedPassword := decryptResetCreds(t, differentLocaleUser, creds, privKey)
	// Double check that the user is in the admin group.
	verifyAdminGroup(t, differentLocaleUser)
	// Verify that the user can log in with the new password.
	verifyLocalWindowsCredentials(ctx, t, differentLocaleUser, decryptedPassword)
}

// verifyAdminGroup verifies that a user is a member of the local admin group.
func verifyAdminGroup(t *testing.T, user string) {
	t.Helper()

	out := verifyPowershellCmd(t, fmt.Sprintf("Get-LocalGroupMember -SID '%s' | ForEach-Object { Write-Output $_.Name}", localAdminSID))
	members := strings.Fields(out)
	t.Logf("Members of admin group: %v", members)
	for _, member := range members {
		if strings.Contains(member, user) {
			return
		}
	}
	t.Errorf("User %s not a member of the admin group (SID %s)", user, localAdminSID)
}
