// Copyright 2026 Google LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     https://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !cit

package guestagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/compute/v1"
)

const (
	readyServicesOutput = "GoogleVssAgent|Running|Automatic\r\n" +
		"GoogleVssProvider|Stopped|Manual\r\n" +
		"GCEAgentManager|Running|Automatic\r\n" +
		"GCEWindowsCompatManager|Running|Automatic\r\n" +
		"GCEAgent|Running|Automatic\r\n" +
		"google_osconfig_agent|Running|Automatic\r\n"

	stableWritersOutput = "vssadmin 1.1 - Volume Shadow Copy Service administrative command-line tool\r\n" +
		"(C) Copyright 2001-2013 Microsoft Corp.\r\n\r\n" +
		"Writer name: 'Task Scheduler Writer'\r\n" +
		"   Writer Id: {d61d61c8-d73a-4eee-8cdd-f6f9786b7124}\r\n" +
		"   Writer Instance Id: {1bddd48e-5052-49db-9b07-b96f96727e6b}\r\n" +
		"   State: [1] Stable\r\n" +
		"   Last error: No error\r\n\r\n" +
		"Writer name: 'System Writer'\r\n" +
		"   Writer Id: {e8132975-6f93-4464-a53e-1050253ae220}\r\n" +
		"   Writer Instance Id: {2a4e91f1-72b0-42aa-9e8a-0f1b2c3d4e5f}\r\n" +
		"   State: [1] Stable\r\n" +
		"   Last error: No error\r\n"

	daisyGuestFlush412FailureWithinGuest = "operation failed &{HttpErrorMessage:PRECONDITION FAILED HttpErrorStatusCode:412 OperationType:createSnapshot Status:DONE}: \n" +
		"        Code: GUEST_FLUSH_FAILED_WITH_DETAILS\n" +
		"        Message: Guest Consistent Snapshot failed (Detail: Failure within guest)."

	daisyGuestFlush412UnhandledReturnCode = "operation failed &{HttpErrorMessage:PRECONDITION FAILED HttpErrorStatusCode:412 OperationType:createSnapshot Status:DONE}: \n" +
		"        Code: GUEST_FLUSH_FAILED_WITH_DETAILS\n" +
		"        Message: Guest Consistent Snapshot failed (Detail: unhandled script return code 0)."
)

type fakeSnapshotCreator struct {
	errs  []error
	calls int
}

func (f *fakeSnapshotCreator) CreateSnapshotWithGuestFlush(project, zone, disk string, s *compute.Snapshot) error {
	f.calls++
	idx := f.calls - 1
	if idx < len(f.errs) {
		return f.errs[idx]
	}
	return nil
}

type logRecorder struct {
	lines []string
}

func (l *logRecorder) logf(format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *logRecorder) joined() string {
	return strings.Join(l.lines, "\n")
}

func (l *logRecorder) countContaining(substr string) int {
	var count int
	for _, line := range l.lines {
		if strings.Contains(line, substr) {
			count++
		}
	}
	return count
}

func TestIsRetryableWindowsGuestFlushError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil_error",
			err:  nil,
			want: false,
		},
		{
			name: "mode_a_failure_within_guest_412",
			err:  errors.New(daisyGuestFlush412FailureWithinGuest),
			want: true,
		},
		{
			name: "mode_b_unhandled_return_code_412",
			err:  errors.New(daisyGuestFlush412UnhandledReturnCode),
			want: true,
		},
		{
			name: "wrapped_guest_flush_412",
			err:  fmt.Errorf("creating snapshot: %w", errors.New(daisyGuestFlush412FailureWithinGuest)),
			want: true,
		},
		{
			name: "googleapi_412_with_guest_flush_code",
			err:  errors.New("googleapi: Error 412: GUEST_FLUSH_FAILED_WITH_DETAILS: timeout, conditionNotMet"),
			want: true,
		},
		{
			name: "other_412_error_code",
			err:  errors.New("operation failed &{HttpErrorMessage:PRECONDITION FAILED HttpErrorStatusCode:412 Status:DONE}: \n        Code: CONDITION_NOT_MET\n        Message: Precondition check failed"),
			want: false,
		},
		{
			name: "googleapi_412_without_guest_flush_code",
			err:  errors.New("googleapi: Error 412: Precondition check failed., conditionNotMet"),
			want: false,
		},
		{
			name: "http_403_permission_denied",
			err:  errors.New("googleapi: Error 403: Required 'compute.disks.createSnapshot' permission, forbidden"),
			want: false,
		},
		{
			name: "http_403_quota_exceeded",
			err:  errors.New("operation failed &{HttpErrorMessage:FORBIDDEN HttpErrorStatusCode:403 Status:DONE}: \n        Code: QUOTA_EXCEEDED\n        Message: Quota 'SNAPSHOTS' exceeded"),
			want: false,
		},
		{
			name: "http_400_bad_request",
			err:  errors.New("googleapi: Error 400: Invalid value for field 'resource.name', invalid"),
			want: false,
		},
		{
			name: "http_404_not_found",
			err:  errors.New("googleapi: Error 404: The resource 'disks/missing' was not found, notFound"),
			want: false,
		},
		{
			name: "non_412_mentioning_guest_flush_code",
			err:  errors.New("operation failed &{HttpErrorStatusCode:500 Status:DONE}: \n        Code: GUEST_FLUSH_FAILED_WITH_DETAILS\n        Message: Internal error"),
			want: false,
		},
		{
			name: "other_code_with_substring_in_message",
			err:  errors.New("operation failed &{HttpErrorStatusCode:412 Status:DONE}: \n        Code: OTHER_ERROR\n        Message: saw Code: GUEST_FLUSH_FAILED_WITH_DETAILS inline"),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableWindowsGuestFlushError(tc.err); got != tc.want {
				t.Errorf("isRetryableWindowsGuestFlushError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestCheckVSSWritersStable(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		wantStable     bool
		wantSummarySub string
	}{
		{
			name:           "all_stable_crlf",
			output:         stableWritersOutput,
			wantStable:     true,
			wantSummarySub: "2 writers stable",
		},
		{
			name: "all_stable_with_prior_timeout_last_error",
			output: "Writer name: 'System Writer'\n" +
				"   State: [1] Stable\n" +
				"   Last error: Timed out\n",
			wantStable:     true,
			wantSummarySub: "prior lastError: System Writer(Timed out)",
		},
		{
			name: "writer_waiting_for_completion",
			output: "Writer name: 'Task Scheduler Writer'\n" +
				"   State: [1] Stable\n" +
				"   Last error: No error\n\n" +
				"Writer name: 'WMI Writer'\n" +
				"   State: [5] Waiting for completion\n" +
				"   Last error: No error\n",
			wantStable:     false,
			wantSummarySub: "WMI Writer state=[5] Waiting for completion",
		},
		{
			name: "writer_waiting_for_freeze",
			output: "Writer name: 'NTDS'\n" +
				"   State: [2] Waiting for freeze\n" +
				"   Last error: No error\n",
			wantStable:     false,
			wantSummarySub: "NTDS state=[2] Waiting for freeze",
		},
		{
			name: "writer_failed_at_freeze_with_error",
			output: "Writer name: 'System Writer'\n" +
				"   State: [9] Failed at freeze\n" +
				"   Last error: Timed out\n",
			wantStable:     false,
			wantSummarySub: "System Writer state=[9] Failed at freeze lastError=Timed out",
		},
		{
			name: "truncated_writer_missing_state",
			output: "Writer name: 'Registry Writer'\n" +
				"   Writer Id: {afbab4a2-367d-4d15-a586-71dbb18f8485}\n",
			wantStable:     false,
			wantSummarySub: "Registry Writer state=unknown",
		},
		{
			name:           "empty_output",
			output:         "",
			wantStable:     false,
			wantSummarySub: "no VSS writers reported",
		},
		{
			name:           "header_only_no_writers",
			output:         "vssadmin 1.1 - Volume Shadow Copy Service\r\nNo items found that satisfy the query.\r\n",
			wantStable:     false,
			wantSummarySub: "no VSS writers reported",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotStable, gotSummary := checkVSSWritersStable(parseVSSWriters(tc.output))
			if gotStable != tc.wantStable {
				t.Errorf("checkVSSWritersStable(%q) stable = %v, want %v (summary=%q)",
					tc.name, gotStable, tc.wantStable, gotSummary)
			}
			if !strings.Contains(gotSummary, tc.wantSummarySub) {
				t.Errorf("checkVSSWritersStable(%q) summary = %q, want substring %q",
					tc.name, gotSummary, tc.wantSummarySub)
			}
		})
	}
}

func TestCheckWindowsServicesReady(t *testing.T) {
	tests := []struct {
		name           string
		output         string
		wantReady      bool
		wantSummarySub string
	}{
		{
			name:           "all_automatic_services_running_manual_provider_stopped",
			output:         readyServicesOutput,
			wantReady:      true,
			wantSummarySub: "GoogleVssAgent=Running(Automatic)",
		},
		{
			name: "optional_osconfig_missing_and_disabled_service_stopped",
			output: "GoogleVssAgent|Running|Automatic\n" +
				"GCEAgentManager|Running|Automatic\n" +
				"GCEWindowsCompatManager|Stopped|Disabled\n" +
				"GCEAgent|Running|Automatic\n",
			wantReady:      true,
			wantSummarySub: "GCEAgent=Running(Automatic)",
		},
		{
			name: "vss_agent_missing",
			output: "GCEAgentManager|Running|Automatic\n" +
				"GCEAgent|Running|Automatic\n",
			wantReady:      false,
			wantSummarySub: "GoogleVssAgent not found",
		},
		{
			name: "vss_agent_stopped_even_when_manual",
			output: "GoogleVssAgent|Stopped|Manual\n" +
				"GCEAgent|Running|Automatic\n",
			wantReady:      false,
			wantSummarySub: "GoogleVssAgent status=Stopped(Manual)",
		},
		{
			name: "delayed_start_manager_and_osconfig_not_yet_running",
			output: "GoogleVssAgent|Running|Automatic\n" +
				"GCEAgentManager|Stopped|Automatic\n" +
				"GCEWindowsCompatManager|StartPending|Automatic\n" +
				"GCEAgent|Running|Automatic\n" +
				"google_osconfig_agent|Stopped|Automatic\n",
			wantReady:      false,
			wantSummarySub: "pending automatic services: GCEAgentManager=Stopped, GCEWindowsCompatManager=StartPending, google_osconfig_agent=Stopped",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotReady, gotSummary := checkWindowsServicesReady(parseWindowsServices(tc.output))
			if gotReady != tc.wantReady {
				t.Errorf("checkWindowsServicesReady(%q) ready = %v, want %v (summary=%q)",
					tc.name, gotReady, tc.wantReady, gotSummary)
			}
			if !strings.Contains(gotSummary, tc.wantSummarySub) {
				t.Errorf("checkWindowsServicesReady(%q) summary = %q, want substring %q",
					tc.name, gotSummary, tc.wantSummarySub)
			}
		})
	}
}

func TestWaitForWindowsReadyImmediateAndEventual(t *testing.T) {
	tests := []struct {
		name           string
		serviceOutputs []string
		writerOutputs  []string
		writerExitCode []int
		wantMinPolls   int
		wantLogSub     string
	}{
		{
			name:           "immediate_ready",
			serviceOutputs: []string{readyServicesOutput},
			writerOutputs:  []string{stableWritersOutput},
			wantMinPolls:   1,
			wantLogSub:     "satisfied on first poll",
		},
		{
			name: "eventual_ready_after_pending_service_and_unstable_writer",
			serviceOutputs: []string{
				"GoogleVssAgent|Running|Automatic\nGCEAgentManager|Stopped|Automatic\n",
				readyServicesOutput,
				readyServicesOutput,
			},
			writerOutputs: []string{
				stableWritersOutput,
				"Writer name: 'System Writer'\n   State: [5] Waiting for completion\n   Last error: No error\n",
				stableWritersOutput,
			},
			wantMinPolls: 3,
			wantLogSub:   "satisfied after 3 poll(s)",
		},
		{
			name: "eventual_ready_after_vssadmin_nonzero_exit",
			serviceOutputs: []string{
				readyServicesOutput,
				readyServicesOutput,
			},
			writerOutputs:  []string{"Error: VSS service starting", stableWritersOutput},
			writerExitCode: []int{1, 0},
			wantMinPolls:   2,
			wantLogSub:     "satisfied after 2 poll(s)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var svcCalls, vssCalls int
			runner := &snapshotRunner{
				isWindows:         true,
				readinessTimeout:  5 * time.Second,
				readinessInterval: time.Millisecond,
				runPS: func(_ context.Context, cmd string) (utils.ProcessStatus, error) {
					if strings.Contains(cmd, "Get-Service") {
						out := tc.serviceOutputs[min(svcCalls, len(tc.serviceOutputs)-1)]
						svcCalls++
						return utils.ProcessStatus{Stdout: out}, nil
					}
					idx := min(vssCalls, len(tc.writerOutputs)-1)
					code := 0
					var err error
					if vssCalls < len(tc.writerExitCode) && tc.writerExitCode[vssCalls] != 0 {
						code = tc.writerExitCode[vssCalls]
						err = fmt.Errorf("exit status %d", code)
					}
					vssCalls++
					return utils.ProcessStatus{Stdout: tc.writerOutputs[idx], Exitcode: code}, err
				},
			}
			var logs logRecorder
			if err := runner.waitForWindowsReady(t.Context(), logs.logf); err != nil {
				t.Fatalf("waitForWindowsReady() returned unexpected error: %v", err)
			}
			if svcCalls < tc.wantMinPolls {
				t.Errorf("waitForWindowsReady() service polls = %d, want >= %d", svcCalls, tc.wantMinPolls)
			}
			if !strings.Contains(logs.joined(), tc.wantLogSub) {
				t.Errorf("waitForWindowsReady() logs = %q, want substring %q", logs.joined(), tc.wantLogSub)
			}
		})
	}
}

func TestWaitForWindowsReadyTimeoutProceedsBestEffort(t *testing.T) {
	runner := &snapshotRunner{
		isWindows:         true,
		readinessTimeout:  15 * time.Millisecond,
		readinessInterval: 3 * time.Millisecond,
		runPS: func(_ context.Context, cmd string) (utils.ProcessStatus, error) {
			if strings.Contains(cmd, "Get-Service") {
				return utils.ProcessStatus{
					Stdout: "GoogleVssAgent|Running|Automatic\nGCEAgentManager|Stopped|Automatic\n",
				}, nil
			}
			return utils.ProcessStatus{
				Stdout: "Writer name: 'WMI Writer'\n   State: [9] Failed at freeze\n   Last error: Timed out\n",
			}, nil
		},
	}

	var logs logRecorder
	if err := runner.waitForWindowsReady(t.Context(), logs.logf); err != nil {
		t.Fatalf("waitForWindowsReady() = %v on best-effort timeout, want nil", err)
	}
	logged := logs.joined()
	for _, wantSub := range []string{
		"Warning: Windows snapshot readiness timed out",
		"proceeding anyway",
		"GCEAgentManager=Stopped(Automatic)",
		"WMI Writer",
	} {
		if !strings.Contains(logged, wantSub) {
			t.Errorf("waitForWindowsReady() timeout log = %q, want substring %q", logged, wantSub)
		}
	}
}

func TestWaitForWindowsReadyContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var polls int
	runner := &snapshotRunner{
		isWindows:         true,
		readinessTimeout:  10 * time.Second,
		readinessInterval: 5 * time.Second,
		runPS: func(_ context.Context, cmd string) (utils.ProcessStatus, error) {
			if strings.Contains(cmd, "Get-Service") {
				polls++
				cancel()
				return utils.ProcessStatus{Stdout: "GoogleVssAgent|Stopped|Automatic\n"}, nil
			}
			return utils.ProcessStatus{Stdout: stableWritersOutput}, nil
		},
	}

	var logs logRecorder
	err := runner.waitForWindowsReady(ctx, logs.logf)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("waitForWindowsReady(canceled ctx) = %v, want errors.Is(_, context.Canceled)", err)
	}
	if polls != 1 {
		t.Errorf("waitForWindowsReady(canceled ctx) polls = %d, want 1", polls)
	}
}

func TestPrepareWindowsSnapshotSleepsPostBootSettle(t *testing.T) {
	var slept []time.Duration
	runner := &snapshotRunner{
		isWindows:           true,
		readinessTimeout:    time.Second,
		readinessInterval:   time.Millisecond,
		postBootSettleDelay: windowsPostBootSettleDelay,
		runPS: func(_ context.Context, cmd string) (utils.ProcessStatus, error) {
			if strings.Contains(cmd, "Get-Service") {
				return utils.ProcessStatus{Stdout: readyServicesOutput}, nil
			}
			return utils.ProcessStatus{Stdout: stableWritersOutput}, nil
		},
		sleep: func(_ context.Context, d time.Duration) error {
			slept = append(slept, d)
			return nil
		},
	}

	var logs logRecorder
	if err := runner.prepareWindowsSnapshot(t.Context(), logs.logf); err != nil {
		t.Fatalf("prepareWindowsSnapshot() returned unexpected error: %v", err)
	}
	wantSlept := []time.Duration{30 * time.Second}
	if diff := cmp.Diff(wantSlept, slept); diff != "" {
		t.Errorf("prepareWindowsSnapshot() sleep durations diff (-want +got):\n%s", diff)
	}
}

func TestCreateSnapshotWithRetryWindows(t *testing.T) {
	errFlushA := errors.New(daisyGuestFlush412FailureWithinGuest)
	errFlushB := errors.New(daisyGuestFlush412UnhandledReturnCode)
	errPerm := errors.New("googleapi: Error 403: Required permission, forbidden")

	attemptTimestamps := []string{
		"2026-11-16T23:00:00.1234567Z",
		"2026-11-16T23:01:30.2345678Z",
		"2026-11-16T23:03:00.3456789Z",
	}

	tests := []struct {
		name               string
		snapshotErrs       []error
		wantErr            bool
		wantAttempts       int
		wantStartUTC       string
		wantSleeps         []time.Duration
		wantReadinessCalls int
		wantDiagDumps      int
	}{
		{
			name:               "succeeds_first_attempt",
			snapshotErrs:       []error{nil},
			wantErr:            false,
			wantAttempts:       1,
			wantStartUTC:       attemptTimestamps[0],
			wantSleeps:         nil,
			wantReadinessCalls: 1,
			wantDiagDumps:      0,
		},
		{
			name:               "succeeds_second_attempt_after_412_mode_a",
			snapshotErrs:       []error{errFlushA, nil},
			wantErr:            false,
			wantAttempts:       2,
			wantStartUTC:       attemptTimestamps[1],
			wantSleeps:         []time.Duration{30 * time.Second},
			wantReadinessCalls: 2,
			wantDiagDumps:      1,
		},
		{
			name:               "succeeds_third_attempt_after_two_412s",
			snapshotErrs:       []error{errFlushA, errFlushB, nil},
			wantErr:            false,
			wantAttempts:       3,
			wantStartUTC:       attemptTimestamps[2],
			wantSleeps:         []time.Duration{30 * time.Second, 30 * time.Second},
			wantReadinessCalls: 3,
			wantDiagDumps:      2,
		},
		{
			name:               "fails_immediately_on_non_retryable_error",
			snapshotErrs:       []error{errPerm, nil},
			wantErr:            true,
			wantAttempts:       1,
			wantStartUTC:       "",
			wantSleeps:         nil,
			wantReadinessCalls: 1,
			wantDiagDumps:      1,
		},
		{
			name:               "exhausts_all_three_attempts_on_persistent_412",
			snapshotErrs:       []error{errFlushA, errFlushB, errFlushA},
			wantErr:            true,
			wantAttempts:       3,
			wantStartUTC:       "",
			wantSleeps:         []time.Duration{30 * time.Second, 30 * time.Second},
			wantReadinessCalls: 3,
			wantDiagDumps:      3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var readinessCalls, utcCalls int
			var sleeps []time.Duration
			runner := &snapshotRunner{
				isWindows:          true,
				readinessTimeout:   time.Second,
				readinessInterval:  time.Millisecond,
				maxWindowsAttempts: windowsSnapshotMaxAttempts,
				retryDelay:         windowsSnapshotRetryDelay,
				diagTimeout:        time.Second,
				sleep: func(_ context.Context, d time.Duration) error {
					sleeps = append(sleeps, d)
					return nil
				},
				runPS: func(_ context.Context, cmd string) (utils.ProcessStatus, error) {
					switch {
					case strings.Contains(cmd, "Get-Service"):
						readinessCalls++
						return utils.ProcessStatus{Stdout: readyServicesOutput}, nil
					case cmd == windowsListWritersCmd:
						return utils.ProcessStatus{Stdout: stableWritersOutput}, nil
					case cmd == windowsCaptureUTCCmd:
						ts := attemptTimestamps[min(utcCalls, len(attemptTimestamps)-1)]
						utcCalls++
						return utils.ProcessStatus{Stdout: ts + "\r\n"}, nil
					case strings.Contains(cmd, vssAgentChannel):
						return utils.ProcessStatus{
							Stdout: "[2026-11-16T23:01:00.0000000Z] Id=1 Level=Information Message=ERROR: Selected writer 'System Writer' is in failed state. Status: 8, Writer Failure code: 0x800423f2",
						}, nil
					case strings.Contains(cmd, vssProviderChannel):
						return utils.ProcessStatus{Stdout: "event log channel not found"}, nil
					case strings.Contains(cmd, "VolSnap"):
						return utils.ProcessStatus{
							Stdout: "[2026-11-16T23:01:00.0000000Z] Provider=VSS Id=12293 Level=Error Message=Shadow copy error",
						}, nil
					case strings.Contains(cmd, "vssadmin"):
						return utils.ProcessStatus{Stdout: stableWritersOutput}, nil
					default:
						return utils.ProcessStatus{}, fmt.Errorf("unexpected PowerShell command: %s", cmd)
					}
				},
			}

			client := &fakeSnapshotCreator{errs: tc.snapshotErrs}
			var logs logRecorder
			gotUTC, gotAttempts, err := runner.createSnapshotWithRetry(
				t.Context(), client, "test-proj", "us-central1-staginga", "vm-1", logs.logf)

			if (err != nil) != tc.wantErr {
				t.Fatalf("createSnapshotWithRetry() error = %v, wantErr %v", err, tc.wantErr)
			}
			if gotAttempts != tc.wantAttempts || client.calls != tc.wantAttempts {
				t.Errorf("createSnapshotWithRetry() attempts = %d (client calls=%d), want %d",
					gotAttempts, client.calls, tc.wantAttempts)
			}
			if gotUTC != tc.wantStartUTC {
				t.Errorf("createSnapshotWithRetry() startUTC = %q, want %q", gotUTC, tc.wantStartUTC)
			}
			if diff := cmp.Diff(tc.wantSleeps, sleeps); diff != "" {
				t.Errorf("createSnapshotWithRetry() sleep durations diff (-want +got):\n%s", diff)
			}
			if readinessCalls != tc.wantReadinessCalls {
				t.Errorf("createSnapshotWithRetry() readiness checks = %d, want %d", readinessCalls, tc.wantReadinessCalls)
			}
			for _, diagLabel := range []string{
				vssAgentChannel + "]:",
				vssProviderChannel + "]:",
				"Application(VSS/VolSnap)]:",
				"vssadmin list writers]:",
			} {
				if got := logs.countContaining(diagLabel); got != tc.wantDiagDumps {
					t.Errorf("createSnapshotWithRetry() diagnostic %q count = %d, want %d",
						diagLabel, got, tc.wantDiagDumps)
				}
			}
		})
	}
}

func TestCreateSnapshotWithRetryDiagnosticErrorsDoNotMaskSnapshotFailure(t *testing.T) {
	wantErr := errors.New(daisyGuestFlush412FailureWithinGuest)
	runner := &snapshotRunner{
		isWindows:          true,
		readinessTimeout:   time.Second,
		readinessInterval:  time.Millisecond,
		maxWindowsAttempts: 1,
		retryDelay:         time.Millisecond,
		diagTimeout:        time.Second,
		sleep:              func(context.Context, time.Duration) error { return nil },
		runPS: func(_ context.Context, cmd string) (utils.ProcessStatus, error) {
			switch {
			case strings.Contains(cmd, "Get-Service"):
				return utils.ProcessStatus{Stdout: readyServicesOutput}, nil
			case cmd == windowsListWritersCmd:
				return utils.ProcessStatus{Stdout: stableWritersOutput}, nil
			case cmd == windowsCaptureUTCCmd:
				return utils.ProcessStatus{Stdout: "2026-11-16T23:00:00.0000000Z"}, nil
			default:
				return utils.ProcessStatus{Stderr: "channel unavailable"}, errors.New("exit status 1")
			}
		},
	}

	client := &fakeSnapshotCreator{errs: []error{wantErr}}
	var logs logRecorder
	_, gotAttempts, err := runner.createSnapshotWithRetry(
		t.Context(), client, "proj", "zone", "inst", logs.logf)
	if !errors.Is(err, wantErr) {
		t.Errorf("createSnapshotWithRetry() error = %v, want %v", err, wantErr)
	}
	if gotAttempts != 1 {
		t.Errorf("createSnapshotWithRetry() attempts = %d, want 1", gotAttempts)
	}
	if got := logs.countContaining("command returned error (non-fatal)"); got != 4 {
		t.Errorf("createSnapshotWithRetry() non-fatal diag error logs = %d, want 4", got)
	}
}

func TestCreateSnapshotWithRetryContextCanceledDuringDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	runner := &snapshotRunner{
		isWindows:          true,
		readinessTimeout:   time.Second,
		readinessInterval:  time.Millisecond,
		maxWindowsAttempts: windowsSnapshotMaxAttempts,
		retryDelay:         windowsSnapshotRetryDelay,
		diagTimeout:        time.Second,
		sleep: func(_ context.Context, _ time.Duration) error {
			cancel()
			return context.Canceled
		},
		runPS: func(_ context.Context, cmd string) (utils.ProcessStatus, error) {
			switch {
			case strings.Contains(cmd, "Get-Service"):
				return utils.ProcessStatus{Stdout: readyServicesOutput}, nil
			case cmd == windowsCaptureUTCCmd:
				return utils.ProcessStatus{Stdout: "2026-11-16T23:00:00.0000000Z"}, nil
			default:
				return utils.ProcessStatus{Stdout: stableWritersOutput}, nil
			}
		},
	}

	client := &fakeSnapshotCreator{errs: []error{errors.New(daisyGuestFlush412FailureWithinGuest), nil}}
	var logs logRecorder
	_, gotAttempts, err := runner.createSnapshotWithRetry(
		ctx, client, "proj", "zone", "inst", logs.logf)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("createSnapshotWithRetry() error = %v, want errors.Is(_, context.Canceled)", err)
	}
	if gotAttempts != 1 || client.calls != 1 {
		t.Errorf("createSnapshotWithRetry() attempts = %d (client calls=%d), want 1",
			gotAttempts, client.calls)
	}
}

func TestCreateSnapshotWithRetryLinuxSingleAttempt(t *testing.T) {
	tests := []struct {
		name        string
		snapshotErr error
		wantErr     bool
	}{
		{
			name:        "linux_succeeds_single_attempt",
			snapshotErr: nil,
			wantErr:     false,
		},
		{
			name:        "linux_does_not_retry_guest_flush_412",
			snapshotErr: errors.New(daisyGuestFlush412FailureWithinGuest),
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var psCalls, sleepCalls int
			runner := &snapshotRunner{
				isWindows:          false,
				maxWindowsAttempts: windowsSnapshotMaxAttempts,
				runPS: func(context.Context, string) (utils.ProcessStatus, error) {
					psCalls++
					return utils.ProcessStatus{}, errors.New("PowerShell must not run on Linux")
				},
				sleep: func(context.Context, time.Duration) error {
					sleepCalls++
					return nil
				},
			}
			client := &fakeSnapshotCreator{errs: []error{tc.snapshotErr}}
			var logs logRecorder
			gotUTC, gotAttempts, err := runner.createSnapshotWithRetry(
				t.Context(), client, "proj", "us-central1-a", "linux-vm", logs.logf)

			if (err != nil) != tc.wantErr {
				t.Errorf("createSnapshotWithRetry() error = %v, wantErr %v", err, tc.wantErr)
			}
			if gotAttempts != 1 || client.calls != 1 {
				t.Errorf("createSnapshotWithRetry() attempts = %d (client calls=%d), want 1",
					gotAttempts, client.calls)
			}
			if gotUTC != "" {
				t.Errorf("createSnapshotWithRetry() startUTC = %q, want empty on Linux", gotUTC)
			}
			if psCalls != 0 || sleepCalls != 0 {
				t.Errorf("createSnapshotWithRetry() psCalls=%d sleepCalls=%d, want 0 and 0", psCalls, sleepCalls)
			}
		})
	}
}

func TestVerifyWindowsSnapshotLogs(t *testing.T) {
	type fakeEvent struct {
		ts  time.Time
		msg string
	}
	attempt1Time := time.Date(2026, 11, 16, 23, 0, 10, 0, time.UTC)
	attempt2Start := "2026-11-16T23:01:30.5000000Z"
	attempt2Time := time.Date(2026, 11, 16, 23, 1, 35, 0, time.UTC)

	fullSuccessEvents := func(ts time.Time) []fakeEvent {
		return []fakeEvent{
			{ts: ts, msg: "CheckSelectedWriterStatus returned with 0"},
			{ts: ts.Add(time.Second), msg: "PrepareVolumes return status 0"},
			{ts: ts.Add(2 * time.Second), msg: "Creating the shadow in DoSnapshotSet."},
			{ts: ts.Add(3 * time.Second), msg: "DoSnapshotSet aysnc operation completed."},
		}
	}

	tests := []struct {
		name          string
		sinceUTC      string
		events        []fakeEvent
		wantFilterSub string
		wantMissing   []string
		wantErr       bool
	}{
		{
			name:          "winning_retry_has_all_expected_messages",
			sinceUTC:      attempt2Start,
			events:        append(fullSuccessEvents(attempt1Time), fullSuccessEvents(attempt2Time)...),
			wantFilterSub: `@SystemTime>='2026-11-16T23:01:30.5000000Z'`,
			wantMissing:   nil,
		},
		{
			name:     "ignores_messages_from_earlier_failed_attempt",
			sinceUTC: attempt2Start,
			events: append(fullSuccessEvents(attempt1Time),
				fakeEvent{ts: attempt2Time, msg: "PrepareVolumes return status 0"},
				fakeEvent{ts: attempt2Time, msg: "Creating the shadow in DoSnapshotSet."},
			),
			wantFilterSub: `@SystemTime>='2026-11-16T23:01:30.5000000Z'`,
			wantMissing: []string{
				"DoSnapshotSet aysnc operation completed.",
				"CheckSelectedWriterStatus returned with 0",
			},
		},
		{
			name:          "converts_offset_timestamp_to_utc_in_xpath",
			sinceUTC:      "2026-11-16T15:01:30.5-08:00",
			events:        fullSuccessEvents(attempt2Time),
			wantFilterSub: `@SystemTime>='2026-11-16T23:01:30.5000000Z'`,
			wantMissing:   nil,
		},
		{
			name:          "fallback_unscoped_when_since_empty",
			sinceUTC:      "",
			events:        fullSuccessEvents(attempt1Time),
			wantFilterSub: "-LogName 'GCE-VSS-Agent/Operational' -Oldest",
			wantMissing:   nil,
		},
		{
			name:     "rejects_invalid_timestamp",
			sinceUTC: "not-a-timestamp",
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedCmd string
			runner := &snapshotRunner{
				isWindows: true,
				runPS: func(_ context.Context, cmd string) (utils.ProcessStatus, error) {
					capturedCmd = cmd
					cutoff := extractSystemTimeCutoff(cmd)
					var lines []string
					for _, ev := range tc.events {
						if !cutoff.IsZero() && ev.ts.Before(cutoff) {
							continue
						}
						lines = append(lines, fmt.Sprintf("[%s] Id=1 Level=Information Message=%s",
							ev.ts.UTC().Format(utcSystemTimeLayout), ev.msg))
					}
					return utils.ProcessStatus{Stdout: strings.Join(lines, "\n")}, nil
				},
			}

			var logs logRecorder
			gotMissing, err := runner.verifyWindowsSnapshotLogs(t.Context(), tc.sinceUTC, logs.logf)
			if (err != nil) != tc.wantErr {
				t.Fatalf("verifyWindowsSnapshotLogs(%q) error = %v, wantErr %v", tc.sinceUTC, err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !strings.Contains(capturedCmd, tc.wantFilterSub) {
				t.Errorf("verifyWindowsSnapshotLogs(%q) query = %q, want substring %q",
					tc.sinceUTC, capturedCmd, tc.wantFilterSub)
			}
			if diff := cmp.Diff(tc.wantMissing, gotMissing); diff != "" {
				t.Errorf("verifyWindowsSnapshotLogs(%q) missing messages diff (-want +got):\n%s",
					tc.sinceUTC, diff)
			}
		})
	}
}

func TestVerifyWindowsSnapshotLogsCommandError(t *testing.T) {
	wantErr := errors.New("event log query failed")
	runner := &snapshotRunner{
		isWindows: true,
		runPS: func(context.Context, string) (utils.ProcessStatus, error) {
			return utils.ProcessStatus{Stderr: "channel disabled"}, wantErr
		},
	}

	var logs logRecorder
	_, err := runner.verifyWindowsSnapshotLogs(t.Context(), "2026-11-16T22:01:00.0000000Z", logs.logf)
	if !errors.Is(err, wantErr) {
		t.Fatalf("verifyWindowsSnapshotLogs() error = %v, want errors.Is(_, %v)", err, wantErr)
	}
	if !strings.Contains(err.Error(), "channel disabled") {
		t.Errorf("verifyWindowsSnapshotLogs() error = %q, want stderr details", err.Error())
	}
}

func TestCaptureWindowsUTCFallbackOnErrors(t *testing.T) {
	testCases := []struct {
		name   string
		status utils.ProcessStatus
		err    error
	}{
		{
			name:   "powershell_command_fails",
			status: utils.ProcessStatus{Stderr: "access denied"},
			err:    errors.New("exit status 1"),
		},
		{
			name:   "unparseable_timestamp_output",
			status: utils.ProcessStatus{Stdout: "not-a-timestamp\r\n"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runner := &snapshotRunner{
				isWindows:   true,
				diagTimeout: time.Second,
				runPS: func(context.Context, string) (utils.ProcessStatus, error) {
					return tc.status, tc.err
				},
			}
			var logs logRecorder
			if got := runner.captureWindowsUTC(t.Context(), logs.logf); got != "" {
				t.Errorf("captureWindowsUTC() = %q, want empty fallback on error", got)
			}
			if !strings.Contains(logs.joined(), "Warning:") {
				t.Errorf("captureWindowsUTC() logs = %q, want non-fatal warning", logs.joined())
			}
		})
	}
}

func TestLogWindowsVSSDiagnosticsSkipsCanceledContext(t *testing.T) {
	psCalls := 0
	runner := &snapshotRunner{
		isWindows:   true,
		diagTimeout: time.Second,
		runPS: func(context.Context, string) (utils.ProcessStatus, error) {
			psCalls++
			return utils.ProcessStatus{}, nil
		},
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var logs logRecorder
	runner.logWindowsVSSDiagnostics(ctx, 1, 3, logs.logf)
	if psCalls != 0 {
		t.Errorf("PowerShell calls on canceled context = %d, want 0", psCalls)
	}
	if !strings.Contains(logs.joined(), "Skipping Windows VSS diagnostics") {
		t.Errorf("logs = %q, want skip notice", logs.joined())
	}
}

func TestNewSnapshotRunnerAndSleepWithContext(t *testing.T) {
	r := newSnapshotRunner()
	if r.readinessTimeout != windowsReadinessTimeout || r.readinessInterval != windowsReadinessInterval {
		t.Errorf("newSnapshotRunner() readiness = (%v, %v), want (%v, %v)",
			r.readinessTimeout, r.readinessInterval, windowsReadinessTimeout, windowsReadinessInterval)
	}
	if r.postBootSettleDelay != windowsPostBootSettleDelay || r.maxWindowsAttempts != windowsSnapshotMaxAttempts || r.retryDelay != windowsSnapshotRetryDelay {
		t.Errorf("newSnapshotRunner() retry/settle = (%v, %d, %v), want (%v, %d, %v)",
			r.postBootSettleDelay, r.maxWindowsAttempts, r.retryDelay,
			windowsPostBootSettleDelay, windowsSnapshotMaxAttempts, windowsSnapshotRetryDelay)
	}
	if r.runPS == nil || r.sleep == nil {
		t.Fatal("newSnapshotRunner() returned nil runPS or sleep hook")
	}

	if err := sleepWithContext(t.Context(), 0); err != nil {
		t.Errorf("sleepWithContext(ctx, 0) error = %v, want nil", err)
	}
	if err := sleepWithContext(t.Context(), time.Millisecond); err != nil {
		t.Errorf("sleepWithContext(ctx, 1ms) error = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := sleepWithContext(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("sleepWithContext(canceledCtx, 1h) error = %v, want context.Canceled", err)
	}
}

func extractSystemTimeCutoff(cmd string) time.Time {
	const prefix = "@SystemTime>='"
	idx := strings.Index(cmd, prefix)
	if idx < 0 {
		return time.Time{}
	}
	rest := cmd[idx+len(prefix):]
	end := strings.IndexByte(rest, '\'')
	if end < 0 {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, rest[:end])
	if err != nil {
		return time.Time{}
	}
	return ts
}
