// Copyright 2026 Google LLC
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

package guestagent

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"golang.org/x/crypto/ssh"
)

//go:embed scripts/*.ps1
var dcScripts embed.FS

const (
	dcPromotedMarkerPath           = `C:\ProgramData\Google\Compute Engine\cit_dc_promoted.txt`
	dcPromotionAttemptedSentinel   = "PROMOTION_ATTEMPTED"
	windowsInstanceConfigsPath     = `C:\Program Files\Google\Compute Engine\instance_configs.cfg`
	dcDomainName                   = "citdc.internal"
	dcDomainDN                     = "DC=citdc,DC=internal"
	dcDomainNetBIOS                = "CITDC"
	dcPasswordLength               = 20
	adAccountsScriptPath           = "scripts/ad_accounts.ps1"
	checkPublicKeysRegScriptPath   = "scripts/check_public_keys_reg.ps1"
	ensureDCDnsScriptPath          = "scripts/ensure_dc_dns.ps1"
	promoteDCScriptPath            = "scripts/promote_dc.ps1"
	queryAgentEventsScriptPath     = "scripts/query_agent_events.ps1"
	ntdsServiceName                = "NTDS"
	netlogonServiceName            = "Netlogon"
	agentLogEventID                = "882"
	corePluginEventProvider        = "google_guest_agent"
	legacyAgentEventProvider       = "GCEGuestAgent"
	adReadinessTimeout             = 3 * time.Minute
	dcDNSForwardingTimeout         = 2 * time.Minute
	sshRefusalEventTimeout         = 90 * time.Second
	adAdminPollTimeout             = 90 * time.Second
	agentRestartTimeout            = time.Minute
	publicKeyRegPollTimeout        = 30 * time.Second
	dcRefusalSubstring             = "refusing to change Windows accounts on an Active Directory domain controller"
	dcOptInSubstring               = "allow_on_domain_controller = true"
	winProductTypeDomainCtrl       = "2"
	winProductTypeServer           = "3"
	windowsAdminUser               = "Administrator"
	sshKeysMetadataKey             = "ssh-keys"
	allowOnDCHyphenMetadataKey     = "allow-on-domain-controller"
	allowOnDCUnderscoreMetadataKey = "allow_on_domain_controller"
)

func redactScriptOutput(s string, params map[string]string) string {
	out := strings.TrimSpace(s)
	for _, key := range []string{"Password", "AdminPassword", "SafeModePassword"} {
		out = redactSecret(out, params[key])
	}
	return out
}

func verifyPowershellScript(ctx context.Context, t *testing.T, scriptPath string, params map[string]string) string {
	t.Helper()
	procStatus, err := utils.RunPowershellScript(ctx, dcScripts, scriptPath, params)
	if err != nil || procStatus.Exitcode != 0 {
		t.Fatalf("script %s failed (exitcode=%d, err=%v): stdout=%s, stderr=%s", scriptPath, procStatus.Exitcode, err, redactScriptOutput(procStatus.Stdout, params), redactScriptOutput(procStatus.Stderr, params))
	}
	return procStatus.Stdout
}

func ensureDCDNSForwarding(ctx context.Context, t *testing.T) {
	t.Helper()
	verifyPowershellScript(ctx, t, ensureDCDnsScriptPath, map[string]string{
		"RequireRunningDns": "false",
	})
}

func waitForDCDNSForwarding(ctx context.Context, t *testing.T) {
	t.Helper()
	err := utils.PollUntil(ctx, "AD DNS Server forwarder and hostname resolution", agentPollInterval, dcDNSForwardingTimeout, func() (bool, string) {
		procStatus, err := utils.RunPowershellScript(ctx, dcScripts, ensureDCDnsScriptPath, map[string]string{
			"RequireRunningDns": "true",
		})
		if err != nil || procStatus.Exitcode != 0 {
			return false, fmt.Sprintf("err=%v, stderr=%q", err, strings.TrimSpace(procStatus.Stderr))
		}
		return true, ""
	})
	if err != nil {
		t.Fatal(err)
	}
}

func waitForDomainControllerReady(ctx context.Context, t *testing.T, initialAdminPwd string) {
	t.Helper()
	err := utils.PollUntil(ctx, "Active Directory Domain Services readiness", agentPollInterval, adReadinessTimeout, func() (bool, string) {
		ntdsStatus, ntdsErr := windowsServiceStatus(ntdsServiceName)
		netlogonStatus, netlogonErr := windowsServiceStatus(netlogonServiceName)
		if ntdsErr != nil || netlogonErr != nil {
			return false, fmt.Sprintf("service check error (%s=%v, %s=%v)", ntdsServiceName, ntdsErr, netlogonServiceName, netlogonErr)
		}
		if ntdsStatus != serviceRunningStatus || netlogonStatus != serviceRunningStatus {
			return false, fmt.Sprintf("%s=%q, %s=%q", ntdsServiceName, ntdsStatus, netlogonServiceName, netlogonStatus)
		}
		procStatus, err := utils.RunPowershellScript(ctx, dcScripts, adAccountsScriptPath, map[string]string{
			"Action":         "Validate",
			"DomainDN":       dcDomainDN,
			"SamAccountName": windowsAdminUser,
			"Password":       initialAdminPwd,
		})
		if err != nil || procStatus.Exitcode != 0 {
			return false, fmt.Sprintf("%s=%q, %s=%q, authErr=%v (stderr=%q)", ntdsServiceName, ntdsStatus, netlogonServiceName, netlogonStatus, err, redactSecret(procStatus.Stderr, initialAdminPwd))
		}
		authResult := strings.TrimSpace(procStatus.Stdout)
		return authResult == "VALID", fmt.Sprintf("%s=%q, %s=%q, auth=%q", ntdsServiceName, ntdsStatus, netlogonServiceName, netlogonStatus, authResult)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeDCAllowConfig(t *testing.T, origCfg []byte, allow bool) {
	t.Helper()
	var buf bytes.Buffer
	if len(origCfg) > 0 {
		buf.Write(origCfg)
		if !bytes.HasSuffix(origCfg, []byte("\n")) {
			buf.WriteString("\r\n")
		}
	}
	fmt.Fprintf(&buf, "[Accounts]\r\nallow_on_domain_controller = %t\r\n", allow)
	if err := os.WriteFile(windowsInstanceConfigsPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", windowsInstanceConfigsPath, err)
	}
}

func clearAccountMetadata(ctx context.Context) error {
	return utils.DeleteMetadata(
		ctx,
		sshKeysMetadataKey,
		windowsKeysMetadataKey,
		allowOnDCHyphenMetadataKey,
		allowOnDCUnderscoreMetadataKey,
	)
}

func verifyDCRefusalCreds(t *testing.T, user string, creds *credsJSON) {
	t.Helper()
	if creds.EncryptedPassword != "" {
		t.Fatalf("creds.EncryptedPassword for %q on DC = non-empty, want empty", user)
	}
	if creds.PasswordFound {
		t.Fatalf("creds.PasswordFound for %q on DC = true, want false", user)
	}
	if !strings.Contains(creds.ErrorMessage, dcRefusalSubstring) || !strings.Contains(creds.ErrorMessage, dcOptInSubstring) {
		t.Fatalf("creds.ErrorMessage for %q on DC = %q, want substrings %q and %q", user, creds.ErrorMessage, dcRefusalSubstring, dcOptInSubstring)
	}
}

func waitForPublicKeyInRegistry(ctx context.Context, t *testing.T, modulus string) {
	t.Helper()
	if modulus == "" {
		t.Fatalf("waitForPublicKeyInRegistry called with empty modulus")
	}
	err := utils.PollUntil(ctx, fmt.Sprintf("refused key modulus %q in PublicKeys registry", modulus), agentPollInterval, publicKeyRegPollTimeout, func() (bool, string) {
		procStatus, err := utils.RunPowershellScript(ctx, dcScripts, checkPublicKeysRegScriptPath, map[string]string{
			"Modulus": modulus,
		})
		if err != nil || procStatus.Exitcode != 0 {
			return false, fmt.Sprintf("err=%v, stderr=%q", err, strings.TrimSpace(procStatus.Stderr))
		}
		out := strings.TrimSpace(procStatus.Stdout)
		return out == "FOUND", fmt.Sprintf("registry=%q", out)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func latestApplicationEventRecordID(ctx context.Context, t *testing.T) int64 {
	t.Helper()
	out := strings.TrimSpace(verifyPowershellScript(ctx, t, queryAgentEventsScriptPath, map[string]string{
		"Action": "LatestRecordId",
	}))
	id, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		t.Fatalf("failed to parse Application event RecordId %q: %v", out, err)
	}
	return id
}

func waitForSSHRefusalEvent(ctx context.Context, t *testing.T, minRecordID int64, username string, wantCorePlugin bool) {
	t.Helper()
	var provider string
	if wantCorePlugin {
		provider = corePluginEventProvider
	} else {
		provider = legacyAgentEventProvider
	}
	// Query the Windows Application Event Log rather than COM1 because in
	// split-agent mode GCEWindowsAgentManager holds COM1 open while both
	// CorePlugin (google_guest_agent) and GCEWindowsAgent (GCEGuestAgent)
	// reliably write EventID 882 entries to the Application Event Log.
	err := utils.PollUntil(ctx, fmt.Sprintf("%s EventLog SSH refusal for %q after RecordId %d", provider, username, minRecordID), agentPollInterval, sshRefusalEventTimeout, func() (bool, string) {
		procStatus, err := utils.RunPowershellScript(ctx, dcScripts, queryAgentEventsScriptPath, map[string]string{
			"Action":      "ListMessages",
			"MinRecordID": strconv.FormatInt(minRecordID, 10),
			"EventID":     agentLogEventID,
			"Provider":    provider,
		})
		if err != nil || procStatus.Exitcode != 0 {
			return false, fmt.Sprintf("query err=%v, stderr=%q", err, strings.TrimSpace(procStatus.Stderr))
		}
		events := strings.TrimSpace(procStatus.Stdout)
		for _, line := range strings.Split(events, "\n") {
			if !strings.Contains(line, dcRefusalSubstring) || !strings.Contains(line, dcOptInSubstring) {
				continue
			}
			if wantCorePlugin && strings.Contains(line, username) {
				return true, ""
			}
			// Legacy GCEWindowsAgent logs "error running createUser: refusing to
			// change Windows accounts..." without repeating the username; minRecordID
			// scopes this check to the single ssh-keys entry added for this subtest.
			if !wantCorePlugin && strings.Contains(line, "error running createUser:") {
				return true, ""
			}
		}
		return false, fmt.Sprintf("recent events=%q", events)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func adUserExists(ctx context.Context, t *testing.T, username string) bool {
	t.Helper()
	out := strings.TrimSpace(verifyPowershellScript(ctx, t, adAccountsScriptPath, map[string]string{
		"Action":         "UserExists",
		"DomainDN":       dcDomainDN,
		"SamAccountName": username,
	}))
	return out == "FOUND"
}

func waitForADAdminUser(ctx context.Context, t *testing.T, username string) {
	t.Helper()
	err := utils.PollUntil(ctx, fmt.Sprintf("AD user %q in Administrators (%s)", username, localAdminSID), agentPollInterval, adAdminPollTimeout, func() (bool, string) {
		procStatus, err := utils.RunPowershellScript(ctx, dcScripts, adAccountsScriptPath, map[string]string{
			"Action":         "InAdminGroup",
			"DomainDN":       dcDomainDN,
			"SamAccountName": username,
			"AdminGroupSID":  localAdminSID,
		})
		if err != nil || procStatus.Exitcode != 0 {
			return false, fmt.Sprintf("err=%v, stderr=%q", err, strings.TrimSpace(procStatus.Stderr))
		}
		out := strings.TrimSpace(procStatus.Stdout)
		return out == "MEMBER", fmt.Sprintf("membership=%q", out)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func verifyADCredentials(ctx context.Context, t *testing.T, username, password string) {
	t.Helper()
	out := strings.TrimSpace(verifyPowershellScript(ctx, t, adAccountsScriptPath, map[string]string{
		"Action":         "Validate",
		"DomainDN":       dcDomainDN,
		"SamAccountName": username,
		"Password":       password,
	}))
	if out != "VALID" {
		t.Fatalf("Active Directory credential validation for %q returned %q, want VALID", username, out)
	}
}

func ensureADAdminPassword(ctx context.Context, t *testing.T, password string) {
	t.Helper()
	verifyPowershellScript(ctx, t, adAccountsScriptPath, map[string]string{
		"Action":         "EnsurePassword",
		"DomainDN":       dcDomainDN,
		"SamAccountName": windowsAdminUser,
		"Password":       password,
	})
}

func generateTestSSHPublicKey(t *testing.T, comment string) string {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, rsaKeyBitSize)
	if err != nil {
		t.Fatalf("failed to generate RSA key for SSH test: %v", err)
	}
	pub, err := ssh.NewPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("failed to create SSH public key: %v", err)
	}
	return fmt.Sprintf("%s %s", strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub))), comment)
}

func promoteWindowsServerToDC(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	if utils.Exists(dcPromotedMarkerPath, utils.TypeFile) {
		t.Fatalf("marker %s exists while Win32_OperatingSystem.ProductType = %q, want %q after reboot", dcPromotedMarkerPath, winProductTypeServer, winProductTypeDomainCtrl)
	}
	// Write a sentinel marker before mutating system state so that if Boot 1
	// fails mid-promotion and CIT reboots the VM with ProductType still "3",
	// Boot 2 fails at the guard above instead of re-promoting and overwriting
	// Boot 1's test result.
	if err := os.MkdirAll(filepath.Dir(dcPromotedMarkerPath), 0700); err != nil {
		t.Fatalf("failed to create marker directory for %s: %v", dcPromotedMarkerPath, err)
	}
	if err := os.WriteFile(dcPromotedMarkerPath, []byte(dcPromotionAttemptedSentinel+"\r\n"), 0600); err != nil {
		t.Fatalf("failed to write DC promotion sentinel %s: %v", dcPromotedMarkerPath, err)
	}

	initialAdminPwd := utils.ValidWindowsPassword(dcPasswordLength)
	safeModePwd := utils.ValidWindowsPassword(dcPasswordLength)

	t.Logf("Installing AD DS and promoting server to forest %s (%s)", dcDomainName, dcDomainNetBIOS)
	verifyPowershellScript(ctx, t, promoteDCScriptPath, map[string]string{
		"AdminUser":         windowsAdminUser,
		"AdminPassword":     initialAdminPwd,
		"DomainName":        dcDomainName,
		"DomainNetbiosName": dcDomainNetBIOS,
		"SafeModePassword":  safeModePwd,
	})

	ensureDCDNSForwarding(ctx, t)

	if err := os.WriteFile(dcPromotedMarkerPath, []byte(initialAdminPwd+"\r\n"), 0600); err != nil {
		t.Fatalf("failed to write DC promotion marker %s: %v", dcPromotedMarkerPath, err)
	}
	t.Log("AD DS forest promotion staged; returning so CIT reboots into Domain Controller mode")
}

func verifyWindowsDCAccountBehavior(t *testing.T) {
	ctx := t.Context()

	rawMarker, err := os.ReadFile(dcPromotedMarkerPath)
	if err != nil {
		t.Fatalf("failed to read DC promotion marker %s: %v", dcPromotedMarkerPath, err)
	}
	if err := os.Remove(dcPromotedMarkerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed to remove DC promotion marker %s: %v", dcPromotedMarkerPath, err)
	}
	initialAdminPwd := strings.TrimSpace(string(rawMarker))
	if initialAdminPwd == "" || initialAdminPwd == dcPromotionAttemptedSentinel {
		t.Fatalf("DC promotion marker %s = %q, want initial %s password", dcPromotedMarkerPath, initialAdminPwd, windowsAdminUser)
	}

	// Wait for AD DS first (which uses local SCM and $env:COMPUTERNAME without DNS),
	// then ensure NIC DNS fallback and the AD DNS Server forwarder to 169.254.169.254
	// are active on Boot 2 once AD-integrated DNS finishes initializing.
	waitForDomainControllerReady(ctx, t, initialAdminPwd)
	waitForDCDNSForwarding(ctx, t)

	// On a promoted Domain Controller, GCEStartup runs wrapper.exe before
	// delayed auto-start services finish starting. Check mode-switch support
	// (which waits for GCEWindowsCompatManager if configured for automatic
	// startup) and ensure the active guest agent is ready before testing.
	canSwitch, err := canSwitchWindowsAgentMode(ctx)
	if err != nil {
		t.Fatalf("failed to check Windows agent mode switch support: %v", err)
	}
	origCoreEnabled := !isWindowsCoreDisabled()
	if !origCoreEnabled && !canSwitch {
		legacyAvailable, err := hasAvailableLegacyWindowsAgent()
		if err != nil {
			t.Fatalf("failed to check legacy Windows agent availability: %v", err)
		}
		if !legacyAvailable {
			t.Fatal("neither CorePlugin nor legacy GCEWindowsAgent is available on promoted Domain Controller")
		}
	}
	waitForWindowsAgentMode(ctx, t, origCoreEnabled)

	origCfg, origCfgErr := os.ReadFile(windowsInstanceConfigsPath)
	if origCfgErr != nil && !errors.Is(origCfgErr, os.ErrNotExist) {
		t.Fatalf("failed to read %s: %v", windowsInstanceConfigsPath, origCfgErr)
	}
	hadOrigCfg := origCfgErr == nil
	for _, line := range strings.Split(string(origCfg), "\n") {
		if strings.EqualFold(strings.TrimSpace(line), "[Accounts]") {
			t.Fatalf("%s unexpectedly already contains an [Accounts] section", windowsInstanceConfigsPath)
		}
	}

	restoreInstanceConfigs := func() error {
		if hadOrigCfg {
			return os.WriteFile(windowsInstanceConfigsPath, origCfg, 0644)
		}
		if err := os.Remove(windowsInstanceConfigsPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}

	client, err := utils.GetDaisyClient(ctx)
	if err != nil {
		t.Fatalf("failed to create compute client: %v", err)
	}
	var origEnableCoreAttr *string
	if canSwitch {
		origEnableCoreAttr, err = currentInstanceMetadataAttr(ctx, client, enableCorePluginKey)
		if err != nil {
			t.Fatalf("failed to read initial %s metadata: %v", enableCorePluginKey, err)
		}
	}

	t.Cleanup(func() {
		if err := clearAccountMetadata(context.Background()); err != nil {
			t.Errorf("failed to delete account metadata keys in cleanup: %v", err)
		}
		if err := restoreInstanceConfigs(); err != nil {
			t.Errorf("failed to restore %s: %v", windowsInstanceConfigsPath, err)
		}
		if canSwitch {
			if err := restoreWindowsCorePluginMode(origCoreEnabled, origEnableCoreAttr); err != nil {
				t.Errorf("failed to restore core plugin mode (%t) in cleanup: %v", origCoreEnabled, err)
			}
		}
		restartCtx, cancelRestart := context.WithTimeout(context.Background(), agentRestartTimeout)
		defer cancelRestart()
		if err := utils.RestartAgent(restartCtx); err != nil {
			t.Errorf("failed to restart guest agent in cleanup: %v", err)
		}
	})

	agentModes := []struct {
		name           string
		wantCorePlugin bool
		refusedWinUser string
		refusedSSHUser string
		allowedSSHUser string
		allowedWinUser string
	}{
		{
			name:           "CorePlugin",
			wantCorePlugin: true,
			refusedWinUser: "dccorewinref",
			refusedSSHUser: "dccoresshref",
			allowedSSHUser: "dccoresshok",
			allowedWinUser: "dccorewinok",
		},
		{
			name:           "LegacyAgent",
			wantCorePlugin: false,
			refusedWinUser: "dclegwinref",
			refusedSSHUser: "dclegsshref",
			allowedSSHUser: "dclegsshok",
			allowedWinUser: "dclegwinok",
		},
	}

	for _, tc := range agentModes {
		t.Run(tc.name, func(t *testing.T) {
			subCtx := t.Context()
			if canSwitch {
				setWindowsCorePluginEnabled(subCtx, t, tc.wantCorePlugin)
			} else if origCoreEnabled != tc.wantCorePlugin {
				t.Skipf("Windows guest agent mode switching via %s is unavailable; skipping %s.", compatManagerServiceName, tc.name)
			}

			// Start with clean metadata, known Administrator password, and default instance_configs.cfg (no DC opt-in).
			ensureADAdminPassword(subCtx, t, initialAdminPwd)
			if err := clearAccountMetadata(subCtx); err != nil {
				t.Fatalf("failed to delete account metadata keys before %s: %v", tc.name, err)
			}
			if err := restoreInstanceConfigs(); err != nil {
				t.Fatalf("failed to reset %s: %v", windowsInstanceConfigsPath, err)
			}
			if err := utils.RestartAgent(subCtx); err != nil {
				t.Fatalf("failed to restart %s with default config: %v", tc.name, err)
			}
			waitForWindowsAgentMode(subCtx, t, tc.wantCorePlugin)

			// 1. Default refusal for windows-keys password reset on existing domain Administrator,
			// confirming the existing Administrator password remains intact.
			adminCreds, _, err := requestWindowsPasswordReset(subCtx, client, windowsAdminUser)
			if err != nil {
				t.Fatalf("%s password reset request failed to produce COM4 response: %v", windowsAdminUser, err)
			}
			verifyDCRefusalCreds(t, windowsAdminUser, adminCreds)
			verifyADCredentials(subCtx, t, windowsAdminUser, initialAdminPwd)

			// 2. Default refusal for windows-keys creation of a new user, and verify the
			// refused key is recorded in the PublicKeys registry so the agent does not
			// retry or spam logs on subsequent 60-second metadata polls.
			newWinCreds, _, err := requestWindowsPasswordReset(subCtx, client, tc.refusedWinUser)
			if err != nil {
				t.Fatalf("%s password reset request failed to produce COM4 response: %v", tc.refusedWinUser, err)
			}
			verifyDCRefusalCreds(t, tc.refusedWinUser, newWinCreds)
			waitForPublicKeyInRegistry(subCtx, t, newWinCreds.Modulus)
			if got := adUserExists(subCtx, t, tc.refusedWinUser); got {
				t.Fatalf("adUserExists(%q) = %t, want false when allow_on_domain_controller is unset", tc.refusedWinUser, got)
			}

			// 3. Default refusal for ssh-keys user creation in the Windows Application Event Log.
			minEventRecordID := latestApplicationEventRecordID(subCtx, t)
			refusedSSHKey := generateTestSSHPublicKey(t, tc.refusedSSHUser)
			if err := utils.UpsertMetadata(subCtx, sshKeysMetadataKey, fmt.Sprintf("%s:%s", tc.refusedSSHUser, refusedSSHKey)); err != nil {
				t.Fatalf("failed to set %s metadata for %s: %v", sshKeysMetadataKey, tc.refusedSSHUser, err)
			}
			waitForSSHRefusalEvent(subCtx, t, minEventRecordID, tc.refusedSSHUser, tc.wantCorePlugin)
			if got := adUserExists(subCtx, t, tc.refusedSSHUser); got {
				t.Fatalf("adUserExists(%q) = %t, want false when allow_on_domain_controller is unset", tc.refusedSSHUser, got)
			}

			// 4. Bypass attempt via instance metadata: setting allow-on-domain-controller
			// and allow_on_domain_controller in metadata must be ignored and still refused.
			if err := utils.UpsertMetadata(subCtx, allowOnDCHyphenMetadataKey, "true"); err != nil {
				t.Fatalf("failed to set %s metadata: %v", allowOnDCHyphenMetadataKey, err)
			}
			if err := utils.UpsertMetadata(subCtx, allowOnDCUnderscoreMetadataKey, "true"); err != nil {
				t.Fatalf("failed to set %s metadata: %v", allowOnDCUnderscoreMetadataKey, err)
			}
			bypassCreds, _, err := requestWindowsPasswordReset(subCtx, client, tc.refusedWinUser)
			if err != nil {
				t.Fatalf("%s metadata-bypass password reset request failed to produce COM4 response: %v", tc.refusedWinUser, err)
			}
			verifyDCRefusalCreds(t, tc.refusedWinUser, bypassCreds)
			if got := adUserExists(subCtx, t, tc.refusedWinUser); got {
				t.Fatalf("adUserExists(%q) = %t, want false when allow_on_domain_controller is only set in metadata", tc.refusedWinUser, got)
			}

			// 5. Setting allow_on_domain_controller = true in instance_configs.cfg without
			// restarting the guest agent must not take effect until the service restarts.
			writeDCAllowConfig(t, origCfg, true)
			noRestartCreds, _, err := requestWindowsPasswordReset(subCtx, client, tc.refusedWinUser)
			if err != nil {
				t.Fatalf("%s password reset before agent restart failed to produce COM4 response: %v", tc.refusedWinUser, err)
			}
			verifyDCRefusalCreds(t, tc.refusedWinUser, noRestartCreds)
			if got := adUserExists(subCtx, t, tc.refusedWinUser); got {
				t.Fatalf("adUserExists(%q) = %t, want false before guest agent restart", tc.refusedWinUser, got)
			}

			// Clear refused and bypass metadata keys before restarting so only opted-in accounts are provisioned.
			if err := clearAccountMetadata(subCtx); err != nil {
				t.Fatalf("failed to delete refused account metadata keys: %v", err)
			}

			// 6. Restart the guest agent so allow_on_domain_controller = true takes effect,
			// and verify both ssh-keys and windows-keys succeed.
			if err := utils.RestartAgent(subCtx); err != nil {
				t.Fatalf("failed to restart %s after enabling allow_on_domain_controller: %v", tc.name, err)
			}
			waitForWindowsAgentMode(subCtx, t, tc.wantCorePlugin)

			allowedSSHKey := generateTestSSHPublicKey(t, tc.allowedSSHUser)
			if err := utils.UpsertMetadata(subCtx, sshKeysMetadataKey, fmt.Sprintf("%s:%s", tc.allowedSSHUser, allowedSSHKey)); err != nil {
				t.Fatalf("failed to set %s metadata for %s: %v", sshKeysMetadataKey, tc.allowedSSHUser, err)
			}
			waitForADAdminUser(subCtx, t, tc.allowedSSHUser)

			allowedWinCreds, privKey, err := requestWindowsPasswordReset(subCtx, client, tc.allowedWinUser)
			if err != nil {
				t.Fatalf("%s opted-in password reset failed to produce COM4 response: %v", tc.allowedWinUser, err)
			}
			decryptedPwd := decryptResetCreds(t, tc.allowedWinUser, allowedWinCreds, privKey)
			waitForADAdminUser(subCtx, t, tc.allowedWinUser)
			verifyADCredentials(subCtx, t, tc.allowedWinUser, decryptedPwd)

			// 7. Re-disable allow_on_domain_controller = false and verify password reset is refused again.
			if err := clearAccountMetadata(subCtx); err != nil {
				t.Fatalf("failed to delete account metadata keys before re-disabling opt-in: %v", err)
			}
			writeDCAllowConfig(t, origCfg, false)
			if err := utils.RestartAgent(subCtx); err != nil {
				t.Fatalf("failed to restart %s after disabling allow_on_domain_controller: %v", tc.name, err)
			}
			waitForWindowsAgentMode(subCtx, t, tc.wantCorePlugin)

			redisabledCreds, _, err := requestWindowsPasswordReset(subCtx, client, tc.allowedWinUser)
			if err != nil {
				t.Fatalf("%s password reset after disabling allow_on_domain_controller failed to produce COM4 response: %v", tc.allowedWinUser, err)
			}
			verifyDCRefusalCreds(t, tc.allowedWinUser, redisabledCreds)
			// Previous password must still be valid because the reset was refused.
			verifyADCredentials(subCtx, t, tc.allowedWinUser, decryptedPwd)
		})
	}
}

// TestWindowsDomainControllerAccounts promotes a Windows Server VM to an Active
// Directory Domain Controller across a reboot and verifies that both the new
// CorePlugin guest agent and the legacy GCEWindowsAgent refuse metadata-driven
// account creation and password resets by default, allow them when
// allow_on_domain_controller = true is set in instance_configs.cfg, and refuse
// them again once allow_on_domain_controller is disabled.
func TestWindowsDomainControllerAccounts(t *testing.T) {
	utils.WindowsOnly(t)
	utils.SkipWindowsClientImages(t)

	productType := strings.TrimSpace(verifyPowershellCmd(t, "(Get-CimInstance -ClassName Win32_OperatingSystem).ProductType"))
	switch productType {
	case winProductTypeServer:
		// The wrapper sets this guest attribute at the end of boot 1; if it is
		// already present we are on boot 2 and boot 1 failed before promoting.
		if _, err := utils.GetMetadata(t.Context(), "instance", "guest-attributes", utils.GuestAttributeTestNamespace, utils.FirstBootGAKey); err == nil {
			t.Fatalf("second boot reached with Win32_OperatingSystem.ProductType = %q; boot 1 did not complete DC promotion", productType)
		} else if !errors.Is(err, utils.ErrMDSEntryNotFound) {
			t.Fatalf("failed to check %s/%s guest attribute: %v", utils.GuestAttributeTestNamespace, utils.FirstBootGAKey, err)
		}
		promoteWindowsServerToDC(t)
	case winProductTypeDomainCtrl:
		verifyWindowsDCAccountBehavior(t)
	default:
		t.Fatalf("Win32_OperatingSystem.ProductType = %q, want %q (standalone server) or %q (domain controller)", productType, winProductTypeServer, winProductTypeDomainCtrl)
	}
}
