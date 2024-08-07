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

package cvm

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	sevabi "github.com/google/go-sev-guest/abi"
	sevclient "github.com/google/go-sev-guest/client"
	checkpb "github.com/google/go-sev-guest/proto/check"
	spb "github.com/google/go-sev-guest/proto/sevsnp"
	sevvalidate "github.com/google/go-sev-guest/validate"
	sevverify "github.com/google/go-sev-guest/verify"
	tdxclient "github.com/google/go-tdx-guest/client"
	ccpb "github.com/google/go-tdx-guest/proto/checkconfig"
	tdxvalidate "github.com/google/go-tdx-guest/validate"
	tdxverify "github.com/google/go-tdx-guest/verify"
)

var sevMsgList = []string{"AMD Secure Encrypted Virtualization (SEV) active", "AMD Memory Encryption Features active: SEV", "Memory Encryption Features active: AMD SEV"}
var sevSnpMsgList = []string{"SEV: SNP guest platform device initialized", "Memory Encryption Features active: SEV SEV-ES SEV-SNP", "Memory Encryption Features active: AMD SEV SEV-ES SEV-SNP"}
var tdxMsgList = []string{"Memory Encryption Features active: TDX", "Memory Encryption Features active: Intel TDX"}
var rebootCmd = []string{"/usr/bin/sudo", "-n", "/sbin/reboot"}

const (
	tdxreportDataBase64String    = "R29vZ2xlJ3MgdG9wIHNlY3JldA=="
	sevsnpreportDataBase64String = "SGVsbG8gU0VWLVNOUAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="
)

func searchDmesg(t *testing.T, matches []string) {
	output, err := exec.Command("dmesg").CombinedOutput()
	if err != nil {
		t.Fatalf("Error: %v", err)
	}
	for _, m := range matches {
		if strings.Contains(string(output), m) {
			return
		}
	}
	t.Fatal("Module not active or found")
}

func reboot() error {
	command := rebootCmd
	cmd := exec.Command(command[0], command[1:]...)
	if _, err := cmd.Output(); err != nil {
		return fmt.Errorf("exec.Command(%v).Output() = %v, want nil", command, err)
	}
	return nil
}

func TestSEVEnabled(t *testing.T) {
	searchDmesg(t, sevMsgList)
}

func TestSEVSNPEnabled(t *testing.T) {
	searchDmesg(t, sevSnpMsgList)
}

func TestTDXEnabled(t *testing.T) {
	searchDmesg(t, tdxMsgList)
}

func TestLiveMigrate(t *testing.T) {
	marker := "/var/lm-test-start"
	if utils.IsWindows() {
		marker = `C:\lm-test-start`
	}
	if _, err := os.Stat(marker); err != nil && !os.IsNotExist(err) {
		t.Fatalf("could not determine if live migrate testing has already started: %v", err)
	} else if err == nil {
		t.Fatal("unexpected reboot during live migrate test")
	}
	err := os.WriteFile(marker, nil, 0777)
	if err != nil {
		t.Fatalf("could not mark beginning of live migrate testing: %v", err)
	}
	ctx := utils.Context(t)
	prj, zone, err := utils.GetProjectZone(ctx)
	if err != nil {
		t.Fatalf("could not find project and zone: %v", err)
	}
	inst, err := utils.GetInstanceName(ctx)
	if err != nil {
		t.Fatalf("could not get instance: %v", err)
	}
	client, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		t.Fatalf("could not make compute api client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	req := &computepb.SimulateMaintenanceEventInstanceRequest{
		Project:  prj,
		Zone:     zone,
		Instance: inst,
	}
	op, err := client.SimulateMaintenanceEvent(ctx, req)
	if err != nil {
		t.Fatalf("could not migrate self: %v", err)
	}
	op.Wait(ctx) // Errors here come from things completely out of our control, such as the availability of a physical machine to take our VM.
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("could not confirm migrate testing has started ok: %v", err)
	}
	_, err = http.Get("https://cloud.google.com/")
	if err != nil {
		t.Errorf("lost network connection after live migration")
	}
}

func TestTDXAttestation(t *testing.T) {
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("couldn't get image from metadata")
	}
	ctx := utils.Context(t)
	// For Ubuntu image, the tdx_guest module was moved to linux-modules-extra package in the 1016 and newer kernels.
	if strings.Contains(image, "ubuntu") {
		kernelVersionCmd := exec.CommandContext(ctx, "uname", "-r")
		kernelVersionOut, err := kernelVersionCmd.CombinedOutput()
		if err != nil {
			t.Fatalf("error getting kernel version: %v", err)
		}
		kernelVersion := strings.TrimSpace(string(kernelVersionOut))
		// Extract the part after the last dot and compare with 1016
		kernelParts := strings.Split(kernelVersion, "-")
		if len(kernelParts) > 1 {
			kernelRevStr := kernelParts[1]
			kernelRev, err := strconv.Atoi(kernelRevStr)
			if err != nil {
				t.Fatalf("error converting kernelVersion to int: %v", err)
			}
			if int(kernelRev) >= 1016 {
				if _, err := exec.CommandContext(ctx, "apt-get", "update", "-y").CombinedOutput(); err != nil {
					t.Fatalf(`exec.CommandContext(ctx, "apt-get", "update", "-y").CombinedOutput() = %v, want nil`, err)
				}
				output1, err := exec.CommandContext(ctx, "apt-get", "install", "-y", "linux-gcp").CombinedOutput()
				if err != nil {
					t.Fatalf(`exec.CommandContext(ctx, "apt-get", "install", "-y", "linux-gcp").CombinedOutput() = %v, want nil`, err)
				}
				output2, err := exec.CommandContext(ctx, "apt-get", "install", "-y", "linux-modules-extra-gcp").CombinedOutput()
				if err != nil {
					t.Fatalf(`exec.CommandContext(ctx, "apt-get", "install", "-y", "linux-modules-extra-gcp").CombinedOutput() = %v, want nil`, err)
				}
				if !strings.Contains(string(output1), "linux-gcp is already the newest version") ||
					!strings.Contains(string(output2), "linux-modules-extra-gcp is already the newest version") {
					if err := reboot(); err != nil {
						t.Fatalf("Reboot error: %v", err)
					}
				}
				if _, err := exec.CommandContext(ctx, "modprobe", "tdx_guest").CombinedOutput(); err != nil {
					t.Fatalf(`exec.CommandContext(ctx, "modprobe", "tdx_guest").CombinedOutput() = %v, want nil`, err)
				}
			}
		}
	}
	decodedBytes, err := base64.StdEncoding.DecodeString(tdxreportDataBase64String)
	if err != nil {
		t.Fatalf("error decoding reportData string: %v", err)
	}
	var reportData [64]byte
	copy(reportData[:], decodedBytes)
	quoteProvider, err := tdxclient.GetQuoteProvider()
	if err != nil {
		t.Fatalf("error getting quote provider: %v", err)
	}
	quote, err := tdxclient.GetQuote(quoteProvider, reportData)
	if err != nil {
		t.Fatalf("error getting quote from the quote provider: %v", err)
	}
	config := &ccpb.Config{
		RootOfTrust: &ccpb.RootOfTrust{},
		Policy:      &ccpb.Policy{HeaderPolicy: &ccpb.HeaderPolicy{}, TdQuoteBodyPolicy: &ccpb.TDQuoteBodyPolicy{}},
	}
	sopts, err := tdxverify.RootOfTrustToOptions(config.RootOfTrust)
	if err != nil {
		t.Fatalf("error converting root of trust to options for verifying the TDX Quote: %v", err)
	}
	if err := tdxverify.TdxQuote(quote, sopts); err != nil {
		t.Fatalf("error verifying the TDX Quote: %v", err)
	}
	opts, err := tdxvalidate.PolicyToOptions(config.Policy)
	if err != nil {
		t.Fatalf("error converting policy to options for validating the TDX Quote: %v", err)
	}
	if err = tdxvalidate.TdxQuote(quote, opts); err != nil {
		t.Fatalf("error validating the TDX Quote: %v", err)
	}
}

func TestSEVSNPAttestation(t *testing.T) {
	ctx := utils.Context(t)
	ensureSevGuestcmd := exec.CommandContext(ctx, "modprobe", "sev-guest")
	if _, err := ensureSevGuestcmd.CombinedOutput(); err != nil {
		t.Fatalf(`exec.CommandContext(ctx, "modprobe", "sev-guest").Output() = %v, want nil`, err)
	}
	// attest
	decodedBytes, err := base64.StdEncoding.DecodeString(sevsnpreportDataBase64String)
	if err != nil {
		t.Fatalf("base64.StdEncoding.DecodeString(sevsnpreportDataBase64String) = %v, want nil", err)
	}
	var reportData [64]byte
	copy(reportData[:], decodedBytes)
	qp, err := sevclient.GetQuoteProvider()
	if err != nil {
		t.Fatalf(`sevclient.GetQuoteProvider() = %v, want nil`, err)
	}
	rawQuote, err := qp.GetRawQuote(reportData)
	if err != nil {
		t.Fatalf(`qp.GetRawQuote(reportData) = %v, want nil`, err)
	}
	// verify
	attestation, err := sevabi.ReportCertsToProto(rawQuote)
	if err != nil {
		t.Fatalf("sevabi.ReportCertsToProto(rawQuote) = %v, want nil", err)
	}
	attestation.Product = &spb.SevProduct{
		Name: spb.SevProduct_SEV_PRODUCT_MILAN,
	}
	config := &checkpb.Config{
		RootOfTrust: &checkpb.RootOfTrust{},
		Policy: &checkpb.Policy{
			Policy:         (1<<17 | 1<<16),
			MinimumVersion: "0.0",
		},
	}
	sopts, err := sevverify.RootOfTrustToOptions(config.RootOfTrust)
	if err != nil {
		t.Fatalf("sevverify.RootOfTrustToOptions(config.RootOfTrust) = %v, want nil", err)
	}
	if err := sevverify.SnpAttestation(attestation, sopts); err != nil {
		t.Fatalf("sevverify.SnpAttestation(attestation, sopts) = %v, want nil", err)
	}
	// validate
	opts, err := sevvalidate.PolicyToOptions(config.Policy)
	if err != nil {
		t.Fatalf("sevvalidate.PolicyToOptions(config.Policy) = %v, want nil", err)
	}
	if err := sevvalidate.SnpAttestation(attestation, opts); err != nil {
		t.Fatalf("sevvalidate.SnpAttestation(attestation, opts) = %v, want nil", err)
	}
}
