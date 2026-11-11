// Copyright 2026 Google LLC.
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

package utils

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	daisyCompute "github.com/GoogleCloudPlatform/compute-daisy/compute"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/compute/v1"
)

const sampleScriptPath = "scripts/sample.ps1"

func strPtr(s string) *string {
	return &s
}

func setupFakeMetadataServer(t *testing.T, project, zone, instance string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Metadata-Flavor") != "Google" {
			http.Error(w, "missing Metadata-Flavor header", http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/computeMetadata/v1/instance/zone":
			fmt.Fprintf(w, "projects/%s/zones/%s", project, zone)
		case "/computeMetadata/v1/instance/name":
			fmt.Fprint(w, instance)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GCE_METADATA_HOST", strings.TrimPrefix(srv.URL, "http://"))
}

func newFakeInstanceClient(t *testing.T, initial *compute.Metadata) (*daisyCompute.TestClient, func() *compute.Metadata, func() int) {
	t.Helper()
	wantProject := "test-proj"
	wantZone := "us-central1-a"
	wantInstance := "test-vm"
	setupFakeMetadataServer(t, wantProject, wantZone, wantInstance)

	currentMD := initial
	setCalls := 0
	ts, tc, err := daisyCompute.NewTestClient(http.NotFound)
	if err != nil {
		t.Fatalf("NewTestClient failed: %v", err)
	}
	t.Cleanup(ts.Close)

	tc.GetInstanceFn = func(project, zone, name string) (*compute.Instance, error) {
		if project != wantProject || zone != wantZone || name != wantInstance {
			return nil, fmt.Errorf("unexpected GetInstance target %s/%s/%s", project, zone, name)
		}
		return &compute.Instance{Metadata: currentMD}, nil
	}
	tc.SetInstanceMetadataFn = func(project, zone, name string, md *compute.Metadata) error {
		if project != wantProject || zone != wantZone || name != wantInstance {
			return fmt.Errorf("unexpected SetInstanceMetadata target %s/%s/%s", project, zone, name)
		}
		setCalls++
		currentMD = md
		return nil
	}
	return tc, func() *compute.Metadata { return currentMD }, func() int { return setCalls }
}

func TestPollUntil(t *testing.T) {
	t.Run("immediate_success", func(t *testing.T) {
		calls := 0
		err := PollUntil(t.Context(), "immediate condition", time.Millisecond, time.Second, func() (bool, string) {
			calls++
			return true, ""
		})
		if err != nil {
			t.Fatalf("PollUntil returned error %v, want nil", err)
		}
		if calls != 1 {
			t.Fatalf("PollUntil invoked check %d times, want 1", calls)
		}
	})

	t.Run("eventual_success", func(t *testing.T) {
		calls := 0
		err := PollUntil(t.Context(), "eventual condition", time.Millisecond, time.Second, func() (bool, string) {
			calls++
			return calls >= 3, fmt.Sprintf("attempt=%d", calls)
		})
		if err != nil {
			t.Fatalf("PollUntil returned error %v, want nil", err)
		}
		if calls != 3 {
			t.Fatalf("PollUntil invoked check %d times, want 3", calls)
		}
	})

	t.Run("timeout_includes_detail", func(t *testing.T) {
		err := PollUntil(t.Context(), "serial response", time.Millisecond, 5*time.Millisecond, func() (bool, string) {
			return false, "offset=42"
		})
		if err == nil {
			t.Fatal("PollUntil succeeded, want timeout error")
		}
		for _, wantSub := range []string{"timed out", "serial response", "offset=42"} {
			if !strings.Contains(err.Error(), wantSub) {
				t.Fatalf("PollUntil error = %q, want substring %q", err.Error(), wantSub)
			}
		}
	})

	t.Run("zero_timeout_checks_once", func(t *testing.T) {
		calls := 0
		err := PollUntil(t.Context(), "zero timeout", time.Second, 0, func() (bool, string) {
			calls++
			return false, ""
		})
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("PollUntil(timeout=0) error = %v, want timeout error", err)
		}
		if calls != 1 {
			t.Fatalf("PollUntil(timeout=0) invoked check %d times, want 1", calls)
		}
	})

	t.Run("precanceled_context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		calls := 0
		err := PollUntil(ctx, "precanceled check", time.Second, time.Minute, func() (bool, string) {
			calls++
			return true, ""
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("PollUntil error = %v, want %v", err, context.Canceled)
		}
		if calls != 0 {
			t.Fatalf("PollUntil invoked check %d times on canceled context, want 0", calls)
		}
	})

	t.Run("context_canceled_during_wait_includes_detail", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		calls := 0
		err := PollUntil(ctx, "active poll", time.Minute, time.Hour, func() (bool, string) {
			calls++
			if calls == 1 {
				time.AfterFunc(10*time.Millisecond, cancel)
			}
			return false, "waiting on service"
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("PollUntil error = %v, want %v", err, context.Canceled)
		}
		if !strings.Contains(err.Error(), "waiting on service") {
			t.Fatalf("PollUntil error = %q, want detail %q", err.Error(), "waiting on service")
		}
	})

	t.Run("invalid_interval_or_timeout_fails_fast", func(t *testing.T) {
		calls := 0
		check := func() (bool, string) {
			calls++
			return true, ""
		}
		if err := PollUntil(t.Context(), "zero interval", 0, time.Second, check); err == nil {
			t.Fatal("PollUntil(interval=0) succeeded, want error")
		}
		if err := PollUntil(t.Context(), "negative interval", -time.Second, time.Second, check); err == nil {
			t.Fatal("PollUntil(interval<0) succeeded, want error")
		}
		if err := PollUntil(t.Context(), "negative timeout", time.Millisecond, -time.Second, check); err == nil {
			t.Fatal("PollUntil(timeout<0) succeeded, want error")
		}
		if calls != 0 {
			t.Fatalf("PollUntil invoked check %d times on invalid parameters, want 0", calls)
		}
	})
}

func TestFilterMetadataItems(t *testing.T) {
	itemA := &compute.MetadataItems{Key: "a", Value: strPtr("1")}
	itemB := &compute.MetadataItems{Key: "b", Value: strPtr("2")}
	itemC := &compute.MetadataItems{Key: "c", Value: strPtr("3")}

	tests := []struct {
		name        string
		items       []*compute.MetadataItems
		keys        []string
		wantItems   []*compute.MetadataItems
		wantRemoved bool
	}{
		{
			name:        "empty_items",
			items:       nil,
			keys:        []string{"a"},
			wantItems:   nil,
			wantRemoved: false,
		},
		{
			name:        "empty_keys",
			items:       []*compute.MetadataItems{itemA, itemB},
			keys:        nil,
			wantItems:   []*compute.MetadataItems{itemA, itemB},
			wantRemoved: false,
		},
		{
			name:        "absent_key",
			items:       []*compute.MetadataItems{itemA, itemB},
			keys:        []string{"missing"},
			wantItems:   []*compute.MetadataItems{itemA, itemB},
			wantRemoved: false,
		},
		{
			name:        "remove_multiple_keys_preserving_order",
			items:       []*compute.MetadataItems{itemA, itemB, itemC},
			keys:        []string{"a", "c"},
			wantItems:   []*compute.MetadataItems{itemB},
			wantRemoved: true,
		},
		{
			name:        "nil_items_preserved",
			items:       []*compute.MetadataItems{itemA, nil, itemB},
			keys:        []string{"a"},
			wantItems:   []*compute.MetadataItems{nil, itemB},
			wantRemoved: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotItems, gotRemoved := filterMetadataItems(tc.items, tc.keys...)
			if gotRemoved != tc.wantRemoved {
				t.Fatalf("filterMetadataItems removed = %t, want %t", gotRemoved, tc.wantRemoved)
			}
			if diff := cmp.Diff(tc.wantItems, gotItems); diff != "" {
				t.Fatalf("filterMetadataItems mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUpsertMetadataWithClient(t *testing.T) {
	t.Run("replaces_and_appends_preserving_fingerprint_and_nil_items", func(t *testing.T) {
		tc, currentMD, setCalls := newFakeInstanceClient(t, &compute.Metadata{
			Fingerprint: "fp-123",
			Items: []*compute.MetadataItems{
				nil,
				{Key: "existing", Value: strPtr("old")},
			},
		})

		if err := upsertMetadataWithClient(t.Context(), tc, "existing", "new"); err != nil {
			t.Fatalf("upsertMetadataWithClient(existing) failed: %v", err)
		}
		if err := upsertMetadataWithClient(t.Context(), tc, "added", "val"); err != nil {
			t.Fatalf("upsertMetadataWithClient(added) failed: %v", err)
		}

		if setCalls() != 2 {
			t.Fatalf("SetInstanceMetadata called %d times, want 2", setCalls())
		}
		wantMD := &compute.Metadata{
			Fingerprint: "fp-123",
			Items: []*compute.MetadataItems{
				nil,
				{Key: "existing", Value: strPtr("new")},
				{Key: "added", Value: strPtr("val")},
			},
		}
		if diff := cmp.Diff(wantMD, currentMD()); diff != "" {
			t.Fatalf("final metadata mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("initializes_nil_instance_metadata", func(t *testing.T) {
		tc, currentMD, setCalls := newFakeInstanceClient(t, nil)

		if err := upsertMetadataWithClient(t.Context(), tc, "windows-keys", "payload"); err != nil {
			t.Fatalf("upsertMetadataWithClient on nil Metadata failed: %v", err)
		}
		if setCalls() != 1 {
			t.Fatalf("SetInstanceMetadata called %d times, want 1", setCalls())
		}
		wantMD := &compute.Metadata{
			Items: []*compute.MetadataItems{
				{Key: "windows-keys", Value: strPtr("payload")},
			},
		}
		if diff := cmp.Diff(wantMD, currentMD()); diff != "" {
			t.Fatalf("metadata mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("propagates_mds_and_client_errors", func(t *testing.T) {
		tc, _, setCalls := newFakeInstanceClient(t, &compute.Metadata{Fingerprint: "fp-123"})

		getErr := errors.New("get instance boom")
		tc.GetInstanceFn = func(_, _, _ string) (*compute.Instance, error) {
			return nil, getErr
		}
		if err := upsertMetadataWithClient(t.Context(), tc, "k", "v"); !errors.Is(err, getErr) {
			t.Fatalf("upsertMetadataWithClient GetInstance error = %v, want %v", err, getErr)
		}
		if setCalls() != 0 {
			t.Fatalf("SetInstanceMetadata called %d times after GetInstance error, want 0", setCalls())
		}

		setErr := errors.New("set metadata boom")
		tc.GetInstanceFn = func(_, _, _ string) (*compute.Instance, error) {
			return &compute.Instance{Metadata: &compute.Metadata{Fingerprint: "fp-123"}}, nil
		}
		tc.SetInstanceMetadataFn = func(_, _, _ string, _ *compute.Metadata) error {
			return setErr
		}
		if err := upsertMetadataWithClient(t.Context(), tc, "k", "v"); !errors.Is(err, setErr) {
			t.Fatalf("upsertMetadataWithClient SetInstanceMetadata error = %v, want %v", err, setErr)
		}

		canceledCtx, cancel := context.WithCancel(t.Context())
		cancel()
		getCalled := false
		tc.GetInstanceFn = func(_, _, _ string) (*compute.Instance, error) {
			getCalled = true
			return &compute.Instance{}, nil
		}
		if err := upsertMetadataWithClient(canceledCtx, tc, "k", "v"); err == nil || !strings.Contains(err.Error(), "failed to get project zone") {
			t.Fatalf("upsertMetadataWithClient MDS error = %v, want 'failed to get project zone' error", err)
		}
		if getCalled {
			t.Fatal("GetInstance called after MDS lookup failed")
		}
	})
}

func TestDeleteMetadataWithClient(t *testing.T) {
	tc, currentMD, setCalls := newFakeInstanceClient(t, &compute.Metadata{
		Fingerprint: "fp-123",
		Items: []*compute.MetadataItems{
			{Key: "keep", Value: strPtr("1")},
			{Key: "drop", Value: strPtr("2")},
		},
	})

	if err := deleteMetadataWithClient(t.Context(), tc); err != nil {
		t.Fatalf("deleteMetadataWithClient() with no keys failed: %v", err)
	}
	if err := deleteMetadataWithClient(t.Context(), tc, "missing-key"); err != nil {
		t.Fatalf("deleteMetadataWithClient(missing-key) failed: %v", err)
	}
	if setCalls() != 0 {
		t.Fatalf("SetInstanceMetadata called %d times when key absent, want 0", setCalls())
	}

	if err := deleteMetadataWithClient(t.Context(), tc, "drop"); err != nil {
		t.Fatalf("deleteMetadataWithClient(drop) failed: %v", err)
	}
	if setCalls() != 1 {
		t.Fatalf("SetInstanceMetadata called %d times after deleting present key, want 1", setCalls())
	}
	wantMD := &compute.Metadata{
		Fingerprint: "fp-123",
		Items: []*compute.MetadataItems{
			{Key: "keep", Value: strPtr("1")},
		},
	}
	if diff := cmp.Diff(wantMD, currentMD()); diff != "" {
		t.Fatalf("metadata after delete mismatch (-want +got):\n%s", diff)
	}
}

func TestPSSingleQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "no_quotes", in: `C:\Temp\cit-ps-123\sample.ps1`, want: `C:\Temp\cit-ps-123\sample.ps1`},
		{name: "single_quote", in: "O'Brien", want: "O''Brien"},
		{name: "adjacent_quotes", in: "a''b'", want: "a''''b''"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PSSingleQuote(tc.in); got != tc.want {
				t.Fatalf("PSSingleQuote(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSortedPowershellParamNames(t *testing.T) {
	t.Run("nil_and_empty_params", func(t *testing.T) {
		got, err := sortedPowershellParamNames(sampleScriptPath, nil)
		if err != nil {
			t.Fatalf("sortedPowershellParamNames(nil) failed: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("sortedPowershellParamNames(nil) = %v, want empty slice", got)
		}
	})

	t.Run("sorts_valid_names", func(t *testing.T) {
		params := map[string]string{
			"Z_Param": "1",
			"_a2":     "2",
			"Alpha":   "3",
		}
		got, err := sortedPowershellParamNames(sampleScriptPath, params)
		if err != nil {
			t.Fatalf("sortedPowershellParamNames(%v) failed: %v", params, err)
		}
		want := []string{"Alpha", "Z_Param", "_a2"}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("sortedPowershellParamNames mismatch (-want +got):\n%s", diff)
		}
	})

	errorTests := []struct {
		name       string
		params     map[string]string
		wantErrSub string
	}{
		{
			name:       "empty_param_name",
			params:     map[string]string{"": "val"},
			wantErrSub: "invalid PowerShell parameter name",
		},
		{
			name:       "leading_digit_param_name",
			params:     map[string]string{"1BadName": "val"},
			wantErrSub: "invalid PowerShell parameter name",
		},
		{
			name:       "hyphenated_param_name",
			params:     map[string]string{"Bad-Name": "val"},
			wantErrSub: "invalid PowerShell parameter name",
		},
		{
			name:       "injection_metachar_param_name",
			params:     map[string]string{"Bad'; exit 1; #": "val"},
			wantErrSub: "invalid PowerShell parameter name",
		},
		{
			name:       "nul_byte_in_param_value",
			params:     map[string]string{"Password": "abc\x00def"},
			wantErrSub: "NUL byte in parameter",
		},
	}

	for _, tc := range errorTests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := sortedPowershellParamNames(sampleScriptPath, tc.params)
			if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("sortedPowershellParamNames(%v) error = %v, want substring %q", tc.params, err, tc.wantErrSub)
			}
		})
	}
}

func TestWriteTempPowershellScript(t *testing.T) {
	wantContent := []byte("Write-Output 'ok'\r\n")
	path, cleanup, err := writeTempPowershellScript(sampleScriptPath, wantContent)
	if err != nil {
		t.Fatalf("writeTempPowershellScript failed: %v", err)
	}

	if filepath.Base(path) != "sample.ps1" {
		cleanup()
		t.Fatalf("filepath.Base(%q) = %q, want %q", path, filepath.Base(path), "sample.ps1")
	}
	gotContent, readErr := os.ReadFile(path)
	if readErr != nil {
		cleanup()
		t.Fatalf("os.ReadFile(%q) failed: %v", path, readErr)
	}
	if diff := cmp.Diff(wantContent, gotContent); diff != "" {
		cleanup()
		t.Fatalf("temp script content mismatch (-want +got):\n%s", diff)
	}

	tmpDir := filepath.Dir(path)
	cleanup()
	if _, statErr := os.Stat(tmpDir); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("os.Stat(%q) after cleanup = %v, want os.ErrNotExist", tmpDir, statErr)
	}
}

func TestRunPowershellScriptValidation(t *testing.T) {
	scripts := fstest.MapFS{
		sampleScriptPath: &fstest.MapFile{
			Data: []byte("Write-Output 'ok'\n"),
		},
	}

	tests := []struct {
		name       string
		scriptPath string
		params     map[string]string
		wantErrSub string
	}{
		{
			name:       "missing_embedded_script",
			scriptPath: "scripts/does_not_exist.ps1",
			params:     nil,
			wantErrSub: "failed to read embedded script",
		},
		{
			name:       "empty_param_name",
			scriptPath: sampleScriptPath,
			params:     map[string]string{"": "val"},
			wantErrSub: "invalid PowerShell parameter name",
		},
		{
			name:       "leading_digit_param_name",
			scriptPath: sampleScriptPath,
			params:     map[string]string{"1BadName": "val"},
			wantErrSub: "invalid PowerShell parameter name",
		},
		{
			name:       "hyphenated_param_name",
			scriptPath: sampleScriptPath,
			params:     map[string]string{"Bad-Name": "val"},
			wantErrSub: "invalid PowerShell parameter name",
		},
		{
			name:       "injection_metachar_param_name",
			scriptPath: sampleScriptPath,
			params:     map[string]string{"Bad'; exit 1; #": "val"},
			wantErrSub: "invalid PowerShell parameter name",
		},
		{
			name:       "nul_byte_in_param_value",
			scriptPath: sampleScriptPath,
			params:     map[string]string{"Password": "abc\x00def"},
			wantErrSub: "NUL byte in parameter",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RunPowershellScript(t.Context(), scripts, tc.scriptPath, tc.params)
			if err == nil {
				t.Fatalf("RunPowershellScript(%q, %v) succeeded, want error containing %q", tc.scriptPath, tc.params, tc.wantErrSub)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("RunPowershellScript(%q, %v) error = %v, want substring %q", tc.scriptPath, tc.params, err, tc.wantErrSub)
			}
		})
	}
}

func TestRunPowershellScriptCanceledContext(t *testing.T) {
	scripts := fstest.MapFS{
		sampleScriptPath: &fstest.MapFile{
			Data: []byte("Write-Output 'ok'\n"),
		},
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	status, err := RunPowershellScript(ctx, scripts, sampleScriptPath, map[string]string{"Valid_Name1": "val"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunPowershellScript with canceled context returned err = %v, want %v", err, context.Canceled)
	}
	if status.Exitcode != -1 {
		t.Fatalf("RunPowershellScript with canceled context returned Exitcode = %d, want -1", status.Exitcode)
	}
}
