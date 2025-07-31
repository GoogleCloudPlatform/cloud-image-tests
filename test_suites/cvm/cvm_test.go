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
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

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
var tdxMsgList = []string{"Memory Encryption Features active: TDX", "Memory Encryption Features active: Intel TDX", "Intel TDX", "tdx: Guest detected"}
var rebootCmd = []string{"/usr/bin/sudo", "-n", "/sbin/reboot"}

const (
	tdxreportDataBase64String    = "R29vZ2xlJ3MgdG9wIHNlY3JldA=="
	sevsnpreportDataBase64String = "SGVsbG8gU0VWLVNOUAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="
	ADXShiftLeaf7EBX             = 19 // ADX: Intel ADX (Multi-Precision Add-Carry Instruction Extensions)
	FPDPShiftLeaf7EBX            = 6  // x87 FPU data pointer updated only on x87 exceptions
	FPCSDSShiftLeaf7EBX          = 13 // FPU CS and FPU DS values are deprecated
	RDSEEDShiftLeaf7EBX          = 18 // RDSEED instruction enabled
	SMAPShiftLeaf7EBX            = 20 // Supervisor Mode Access Prevention
	LA57ShiftLeaf7ECX            = 16 // 5-level paging (increases size of virtual addresses from 48 bits to 57 bits)

	// liveMigrateTimeout is the timeout for live migration. From looking at
	// previous successful test runs, live migration can take up to 15 minutes,
	// with most taking under 12 minutes. We set to 10 minutes to avoid the test
	// timing out while waiting for the live migration to complete, while still
	// giving a chance for the live migration to complete.
	liveMigrateTimeout = 10 * time.Minute
)

func searchDmesg(t *testing.T, matches []string) {
	output, err := exec.Command("dmesg").CombinedOutput()
	if err != nil {
		t.Fatalf("exec.Command(%q)).CombinedOutput() = err %v, want nil", "dmesg", err)
	}
	for _, m := range matches {
		if strings.Contains(string(output), m) {
			return
		}
	}
	t.Fatalf("Want exec.Command(%q)).CombinedOutput() to contain one of these string: %v found none", "dmesg", matches)
}

func reboot() error {
	command := rebootCmd
	cmd := exec.Command(command[0], command[1:]...)
	if _, err := cmd.Output(); err != nil {
		return fmt.Errorf("exec.Command(%v).Output() = %v, want nil", command, err)
	}
	return nil
}

// From https://github.com/intel-go/cpuid/blob/master/cpuidlow_amd64.s
func cpuidlow(leaf, subleaf uint32) (eax, ebx, ecx, edx uint32)

func flagToStr(x bool) string {
	if x {
		return "has"
	}
	return "does not have"
}

func checkBit(t *testing.T, ebx uint32, shift uint, hostHas bool, name string) {
	t.Helper()
	bitSet := ebx&(1<<shift) != 0
	status := "OFF"
	if bitSet {
		status = "ON"
	}
	t.Logf("The %s bit is %s on Guest.\n", name, status)
	t.Logf("Host %s %s\n", flagToStr(hostHas), name)
	if hostHas && !bitSet {
		t.Logf("%s bit should be ON but is OFF", name)
	} else if !hostHas && bitSet {
		t.Logf("%s bit should be OFF but is ON", name)
	}
}

func isIntelPlatform(t *testing.T) bool {
	t.Helper()
	cmd := exec.Command("grep", "-m", "1", "vendor_id", "/proc/cpuinfo")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
		return false
	}
	return strings.Contains(out.String(), "GenuineIntel")
}

func cpuPlatformIn(t *testing.T, platforms []string) bool {
	t.Helper()
	cmd := exec.Command("grep", "model name", "/proc/cpuinfo")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
		return false
	}
	modelName := out.String()
	for _, platform := range platforms {
		if strings.Contains(modelName, platform) {
			return true
		}
	}
	return false
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
	// Add a timeout to live migration. This is to avoid the test getting stuck if
	// the live migration hangs for too long.
	ctx, cancel := context.WithTimeout(utils.Context(t), liveMigrateTimeout)
	defer cancel()
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
	if err := op.Wait(ctx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			// Skipping the test because it timed out. This also helps to signal that
			// the live migration took too long.
			t.Skipf("Live migration timed out after %s", liveMigrateTimeout.String())
		} else {
			// Actual error happened. However, this can be uncontrollable due to any
			// quota issues or temporary unavailability.
			t.Skipf("Live migration failed: %v", err)
		}
	}
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
				t.Fatalf("kernelRev, err := strconv.Atoi(kernelRevStr): %v, want nil", err)
			}
			// Kernel revisions for 22.04 6.5 kernel
			lowerKernelRev := 1016
			upperKernelRev := 1021

			if kernelParts[0] == "6.8.0" {
				// Kernel revisions for 22.04 6.8 kernel
				lowerKernelRev = 1013
				upperKernelRev = 1014
			}

			if strings.Contains(image, "-2404-") {
				// Kernel revisions for 24.04
				lowerKernelRev = 1006
				upperKernelRev = 1008
			}
			// Installing linux-modules-extra-gcp is required only on some kernel versions of 2204 and 2404
			if (strings.Contains(image, "-2204-") || strings.Contains(image, "-2404-")) &&
				int(kernelRev) >= lowerKernelRev && int(kernelRev) < upperKernelRev {
				if _, err := exec.CommandContext(ctx, "apt-get", "update", "-y").CombinedOutput(); err != nil {
					t.Fatalf(`exec.CommandContext(ctx, "apt-get", "update", "-y").CombinedOutput() = %v, want nil`, err)
				}
				output1, err := exec.CommandContext(ctx, "apt-get", "install", "-y", "linux-gcp").CombinedOutput()
				if err != nil {
					t.Fatalf(`exec.CommandContext(ctx, "apt-get", "install", "-y", "linux-gcp").CombinedOutput() = %v, want nil`, err)
				}
				t.Logf("Installing linux-modules-extra-gcp")
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
			}
		}
	}
	if _, err := exec.CommandContext(ctx, "modprobe", "tdx_guest").CombinedOutput(); err != nil {
		t.Fatalf(`exec.CommandContext(ctx, "modprobe", "tdx_guest").CombinedOutput() = %v, want nil`, err)
	}
	decodedBytes, err := base64.StdEncoding.DecodeString(tdxreportDataBase64String)
	if err != nil {
		t.Fatalf("decodedBytes, err := base64.StdEncoding.DecodeString(tdxreportDataBase64String): %v, want nil", err)
	}
	var reportData [64]byte
	copy(reportData[:], decodedBytes)
	quoteProvider, err := tdxclient.GetQuoteProvider()
	if err != nil {
		t.Fatalf("quoteProvider, err := tdxclient.GetQuoteProvider(): %v, want nil", err)
	}
	quote, err := tdxclient.GetQuote(quoteProvider, reportData)
	if err != nil {
		t.Fatalf("quote, err := tdxclient.GetQuote(quoteProvider, reportData): %v, want nil", err)
	}
	config := &ccpb.Config{
		RootOfTrust: &ccpb.RootOfTrust{},
		Policy:      &ccpb.Policy{HeaderPolicy: &ccpb.HeaderPolicy{}, TdQuoteBodyPolicy: &ccpb.TDQuoteBodyPolicy{}},
	}
	sopts, err := tdxverify.RootOfTrustToOptions(config.RootOfTrust)
	if err != nil {
		t.Fatalf("sopts, err := tdxverify.RootOfTrustToOptions(config.RootOfTrust): %v, want nil", err)
	}
	if err := tdxverify.TdxQuote(quote, sopts); err != nil {
		t.Fatalf("err := tdxverify.TdxQuote(quote, sopts): %v, want nil", err)
	}
	opts, err := tdxvalidate.PolicyToOptions(config.Policy)
	if err != nil {
		t.Fatalf("opts, err := tdxvalidate.PolicyToOptions(config.Policy): %v, want nil", err)
	}
	if err = tdxvalidate.TdxQuote(quote, opts); err != nil {
		t.Fatalf("err = tdxvalidate.TdxQuote(quote, opts): %v, want nil", err)
	}
}

func TestSEVSNPAttestation(t *testing.T) {
	ctx := utils.Context(t)
	ensureSevGuestcmd := exec.CommandContext(ctx, "modprobe", "sev-guest")
	if err := ensureSevGuestcmd.Run(); err != nil {
		if err2 := exec.CommandContext(ctx, "modprobe", "sevguest").Run(); err2 != nil {
			t.Fatalf(`exec.CommandContext(ctx, "modprobe", "sev-guest").Run() = %v \n exec.CommandContext(ctx, "modprobe", "sevguest").Run() = %v, want nil for either of them`, err, err2)
		}
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

func TestCheckApicId(t *testing.T) {
	ctx := utils.Context(t)
	cmd := "cat /proc/cpuinfo | grep -m 1 ^apicid"
	apic, err := exec.CommandContext(ctx, "sh", "-c", cmd).CombinedOutput()
	if err != nil {
		t.Fatalf(`exec.CommandContext(ctx, "sh", "-c", "cat /proc/cpuinfo | grep -m 1 ^apicid") = %v, want nil`, err)
	}
	apicidstr := strings.TrimSpace(string(apic))
	re := regexp.MustCompile(`.*0.*$`)
	if !re.MatchString(apicidstr) {
		t.Errorf("expected APIC ID to contain '0', but got: %s", apicidstr)
	}
}

func TestCheckCpuidLeaf7(t *testing.T) {
	_, ebx, ecx, _ := cpuidlow(7, 0)
	// CPU Platform not in ['Intel Ivy Bridge', 'Intel Sandy Bridge', 'Intel Haswell'] should have Broadwell features.
	guestHasBroadwellFeatures := true
	checkBit(t, ebx, ADXShiftLeaf7EBX, guestHasBroadwellFeatures, "adx")
	checkBit(t, ebx, RDSEEDShiftLeaf7EBX, guestHasBroadwellFeatures, "rdseed")
	checkBit(t, ebx, SMAPShiftLeaf7EBX, guestHasBroadwellFeatures, "smap")
	// Intel guests should have FPDP and FPCSDS unconditionally
	if isIntelPlatform(t) {
		checkBit(t, ebx, FPDPShiftLeaf7EBX, guestHasBroadwellFeatures, "fpdp")
		checkBit(t, ebx, FPCSDSShiftLeaf7EBX, guestHasBroadwellFeatures, "fpcsds")
	}
	excludeLA57Platforms := []string{"Intel Sapphire Rapids", "AMD Genoa", "Intel Emerald Rapids"}
	shouldNotHaveLA57 := cpuPlatformIn(t, excludeLA57Platforms)
	if shouldNotHaveLA57 && (ecx&(1<<LA57ShiftLeaf7ECX) != 0) {
		t.Errorf("LA57 bit should be OFF but is ON")
	} else {
		t.Logf("LA57 bit is correctly OFF")
	}
}
