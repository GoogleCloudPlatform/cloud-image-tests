// Copyright 2023 Google LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     https://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mdsmtls

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// checkCredsPresent checks mTLS creds exist on Linux based OSs.
// metadata-script-runner has service dependency and is guaranteed to run after guest-agent.
func checkCredsPresent(t *testing.T, shouldExist bool) {
	t.Helper()

	credsDir := "/run/google-mds-mtls"
	creds := []string{filepath.Join(credsDir, "root.crt"), filepath.Join(credsDir, "client.key")}

	for _, f := range creds {
		_, err := os.Stat(f)
		if shouldExist != (err == nil) {
			t.Errorf("os.Stat(%s) failed with error: %v, mTLS creds expected to be present: %t", f, err, shouldExist)
		}
	}
}

func checkCredsPresentInOSTrustStoreWindows(t *testing.T, shouldExist bool) {
	t.Helper()

	status, err := utils.RunPowershellCmd(`Get-ChildItem Cert:\LocalMachine\My | Where-Object { $_.Issuer -like "*google.internal*" }`)
	if err != nil {
		t.Errorf(`Failed to get ChildItem from Cert:\LocalMachine\My: %v`, err)
	}
	if got := len(status.Stdout) > 0; got != shouldExist {
		t.Errorf("mTLS client creds found in the OS trust store: %t, expected: %t, output: %+v", got, shouldExist, status)
	}

	status, err = utils.RunPowershellCmd(`Get-ChildItem Cert:\LocalMachine\Root | Where-Object { $_.Issuer -like "*google.internal*" }`)
	if err != nil {
		t.Errorf(`Failed to get ChildItem from Cert:\LocalMachine\Root: %v`, err)
	}
	if got := len(status.Stdout) > 0; got != shouldExist {
		t.Errorf("mTLS root creds found in the OS trust store: %t, expected: %t, output: %+v", got, shouldExist, status)
	}
}

func checkCredsPresentInOSTrustStore(t *testing.T, shouldExist, enabled bool) {
	t.Helper()

	defaultLocation := "/run/google-mds-mtls/root.crt"
	credsBytes, err := os.ReadFile(defaultLocation)
	if enabled != (err == nil) {
		t.Fatalf("os.ReadFile(%s) failed with error: %v, mTLS creds expected to be present at %q", defaultLocation, err, defaultLocation)
	}
	creds := string(credsBytes)

	knownLocations := []string{"/usr/local/share/ca-certificates/root.crt", "/usr/share/pki/trust/anchors/root.crt", "/etc/pki/ca-trust/source/anchors/root.crt"}
	var found bool
	var foundLocation string
	var allErrors error
	for _, f := range knownLocations {
		got, err := os.ReadFile(f)
		if err != nil {
			allErrors = errors.Join(allErrors, err)
			continue
		}
		if string(got) == creds {
			found = true
			foundLocation = f
			break
		}
	}
	if shouldExist != found {
		t.Errorf("mTLS root creds found: %t, location: %q, expected to be present: %t, all errors: %v", found, foundLocation, shouldExist, allErrors)
	}
}

// checkCredsPresentWindows checks if mTLS creds exist on Windows systems.
// Unlike Linux metadata-script-runner is not guaranteed to run after guest-agent and implements
// a retry logic to avoid timing issues.
func checkCredsPresentWindows(t *testing.T, shouldExist bool) {
	t.Helper()

	credsDir := filepath.Join(os.Getenv("ProgramData"), "Google", "Compute Engine")
	creds := []string{filepath.Join(credsDir, "mds-mtls-root.crt"), filepath.Join(credsDir, "mds-mtls-client.key"), filepath.Join(credsDir, "mds-mtls-client.key.pfx")}

	var lastErrors []string

	// Try to test every 10 sec for max 2 minutes.
	for i := 1; i <= 12; i++ {
		// Reset the list before every retry.
		lastErrors = nil
		for _, f := range creds {
			if _, err := os.Stat(f); err != nil {
				lastErrors = append(lastErrors, fmt.Sprintf("os.Stat(%s) failed with error: %v, mTLS creds expected to be present at %q", f, err, f))
			}
		}

		if len(lastErrors) == 0 {
			break
		}
		time.Sleep(10 * time.Second)
	}

	if shouldExist != (len(lastErrors) == 0) {
		t.Fatalf("mTLS credentials should exist: %v, got: %v", shouldExist, lastErrors)
	}
}

func TestMTLSCredsExists(t *testing.T) {
	ctx := utils.Context(t)
	if _, err := utils.GetMetadata(ctx, "instance", "credentials", "mds-client-certificate"); err != nil {
		t.Errorf("MTLs certs are not available from the MDS: utils.GetMetadata(ctx, instance/credentials/mds-client-certificate) = err %v, want nil", err)
	}

	var rootKeyFile, clientKeyFile string
	if utils.IsWindows() {
		rootKeyFile = filepath.Join(os.Getenv("ProgramData"), "Google", "Compute Engine", "mds-mtls-root.crt")
		clientKeyFile = filepath.Join(os.Getenv("ProgramData"), "Google", "Compute Engine", "mds-mtls-client.key")
		checkCredsPresentWindows(t, true)
		checkCredsPresentInOSTrustStoreWindows(t, false)
	} else {
		rootKeyFile = filepath.Join("/", "run", "google-mds-mtls", "root.crt")
		clientKeyFile = filepath.Join("/", "run", "google-mds-mtls", "client.key")
		checkCredsPresent(t, true)
		checkCredsPresentInOSTrustStore(t, false, true)
	}
	clientCert, clientKey := splitClientKeyFile(t, clientKeyFile)
	certPair, err := tls.LoadX509KeyPair(clientCert, clientKey)
	if err != nil {
		t.Fatal(err)
	}
	rootKey, err := os.ReadFile(rootKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(rootKey)
	client := http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      caCertPool,
				Certificates: []tls.Certificate{certPair},
			},
		},
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://169.254.169.254/computeMetadata/v1/instance/hostname", nil)
	if err != nil {
		t.Fatalf("could not make http request: %v", err)
	}
	req.Header.Add("Metadata-Flavor", "Google")
	resp, err := client.Do(req)
	if resp.TLS == nil {
		t.Errorf("Metadata response was sent unencrypted")
	}
	verifyCurlWithoutCacert(t, false)
}

func verifyCurlWithoutCacert(t *testing.T, shouldWork bool) {
	t.Helper()
	ctx := utils.Context(t)
	cmd := exec.CommandContext(ctx, "curl", "-E", "/run/google-mds-mtls/client.key", "-H", "MetadataFlavor: Google", "https://169.254.169.254/computeMetadata/v1/instance/hostname")
	out, err := cmd.Output()

	if shouldWork != (err == nil) {
		t.Errorf("curl completed with error: [%v], want error: %t", err, !shouldWork)
	}
	if shouldWork && len(out) <= 0 {
		t.Errorf("curl on https mds hostname endpoint output: %q, want non-empty, output: %+v", string(out), out)
	}
}

func verifyInvokeRestMethodWindows(t *testing.T, shouldWork bool) {
	t.Helper()
	script := `$cert = Get-ChildItem Cert:\LocalMachine\My | Where-Object { $_.Issuer -like "*google.internal*" }; Invoke-RestMethod -Uri https://169.254.169.254 -Method Get -Headers @{"Metadata-Flavor"="Google"} -Certificate $cert`
	status, err := utils.RunPowershellCmd(script)
	if shouldWork != (err == nil) {
		t.Errorf("Invoke-RestMethod on https mds endpoint completed with error: [%v], want error: %t", err, !shouldWork)
	}
	if len(status.Stdout) <= 0 {
		t.Errorf("Invoke-RestMethod on https mds hostname endpoint output: %q, want non-empty, output: %+v", string(status.Stdout), status)
	}
}

func TestCredsExistsInOSTrustStore(t *testing.T) {
	if utils.IsWindows() {
		checkCredsPresentInOSTrustStoreWindows(t, true)
		verifyInvokeRestMethodWindows(t, true)
	} else {
		checkCredsPresentInOSTrustStore(t, true, true)
		verifyCurlWithoutCacert(t, true)
	}
}

func TestMTLSJobScheduled(t *testing.T) {
	ctx := utils.Context(t)
	var cmd *exec.Cmd
	coreDisabled := utils.IsCoreDisabled()
	if utils.IsWindows() {
		cmd = exec.CommandContext(ctx, "powershell.exe", "(Get-WinEvent -Providername GCEGuestAgentManager).Message")
		if coreDisabled {
			cmd = exec.CommandContext(ctx, "powershell.exe", "(Get-WinEvent -Providername GCEGuestAgent).Message")
		}
	} else {
		cmd = exec.CommandContext(ctx, "journalctl", "-o", "cat", "-eu", "google-guest-agent-manager")
		if coreDisabled {
			cmd = exec.CommandContext(ctx, "journalctl", "-o", "cat", "-eu", "google-guest-agent")
		}
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("could not get agent output: %v", err)
	}
	if strings.Contains(string(out), "Failed to schedule job MTLS_MDS_Credential_Boostrapper") {
		t.Errorf("MTLS job is not scheduled. Agent logs: %s", string(out))
	}
}

// Splits the client key file which has a certificate and key concatenated into
// two separate files, returning their locations.
func splitClientKeyFile(t *testing.T, clientKeyFile string) (certPath string, keyPath string) {
	t.Helper()
	clientKeyConcatenated, err := os.ReadFile(clientKeyFile)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) = %v, want nil", clientKeyFile, err)
	}
	clientCert, clientKey := pem.Decode(clientKeyConcatenated)
	if clientCert.Type != "CERTIFICATE" {
		t.Fatalf("pem.Decode(%s) returned block of type %s, want CERTIFICATE", clientKeyConcatenated, clientCert.Type)
	}
	certDir := t.TempDir()
	certPath = filepath.Join(certDir, "client.cert")
	keyPath = filepath.Join(certDir, "client.key")
	certFile, err := os.OpenFile(certPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		t.Fatalf("os.OpenFile(%s, os.O_RDWR|os.O_CREATE, 0644) = %v, want nil", certPath, err)
	}
	if err := pem.Encode(certFile, clientCert); err != nil {
		t.Fatalf("pem.Encode(%s, clientCert) = %v, want nil", certPath, err)
	}
	if err := certFile.Close(); err != nil {
		t.Fatalf("certFile.Close() = %v, want nil", err)
	}
	if err := os.WriteFile(keyPath, clientKey, 0644); err != nil {
		t.Fatalf("os.WriteFile(%s, %s, 0644) = %v, want nil", keyPath, clientKey, err)
	}
	return certPath, keyPath
}

func TestDefaultBehavior(t *testing.T) {
	if utils.IsWindows() {
		checkCredsPresentWindows(t, false)
		checkCredsPresentInOSTrustStoreWindows(t, false)
	} else {
		checkCredsPresent(t, false)
		checkCredsPresentInOSTrustStore(t, false, false)
	}
}
