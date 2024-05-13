// Copyright 2021 Google LLC
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

package imagetest

import (
	"testing"

	"github.com/jstemmer/go-junit-report/v2/junit"
)

var (
	testPass = `
=== RUN   TestUpdateNSSwitchConfig
--- PASS: TestUpdateNSSwitchConfig (0.01s)
=== RUN   TestUpdateSSHConfig
--- PASS: TestUpdateSSHConfig (0.02s)
=== RUN   TestUpdatePAMsshd
--- PASS: TestUpdatePAMsshd (0.00s)
=== RUN   TestUpdateGroupConf
--- PASS: TestUpdateGroupConf (0.00s)
PASS
`
	testFail = `
=== RUN   TestAlwaysFails
    main_test.go:47: failed, message: heh
    main_test.go:47: failed, message: heh2
    main_test.go:47: failed, message: heh again
--- FAIL: TestAlwaysFails (0.00s)
=== RUN   TestUpdateNSSwitchConfig
--- PASS: TestUpdateNSSwitchConfig (0.00s)
=== RUN   TestUpdateSSHConfig
--- PASS: TestUpdateSSHConfig (0.00s)
=== RUN   TestUpdatePAMsshd
--- PASS: TestUpdatePAMsshd (0.00s)
=== RUN   TestUpdateGroupConf
--- PASS: TestUpdateGroupConf (0.00s)
FAIL
`
)

func TestConvertToTestSuite(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		output junit.Testsuite
	}{
		{
			name:   "parse_passing_results",
			input:  []string{testPass},
			output: junit.Testsuite{Tests: 4, Time: "0.030"},
		},
		{
			name:   "parse_failing_results",
			input:  []string{testFail},
			output: junit.Testsuite{Tests: 5, Failures: 1, Time: "0.000"},
		},
		{
			name:   "parse_passing_results_twice",
			input:  []string{testPass, testPass},
			output: junit.Testsuite{Tests: 8, Time: "0.060"},
		},
		{
			name:   "parse_passing_and_failing_results",
			input:  []string{testPass, testFail},
			output: junit.Testsuite{Tests: 9, Failures: 1, Time: "0.030"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertToTestSuite(tt.input, "")
			switch {
			case got.Name != tt.output.Name:
				t.Errorf("unexpected Name got: %+v, want: %+v", got.Name, tt.output.Name)
			case got.Tests != tt.output.Tests:
				t.Errorf("unexpected Tests got: %+v, want: %+v", got.Tests, tt.output.Tests)
			case got.Failures != tt.output.Failures:
				t.Errorf("unexpected Failures got: %+v, want: %+v", got.Failures, tt.output.Failures)
			case got.Errors != tt.output.Errors:
				t.Errorf("unexpected Errors got: %+v, want: %+v", got.Errors, tt.output.Errors)
			case got.Disabled != tt.output.Disabled:
				t.Errorf("unexpected Disabled got: %+v, want: %+v", got.Disabled, tt.output.Disabled)
			case got.Skipped != tt.output.Skipped:
				t.Errorf("unexpected Skipped got: %+v, want: %+v", got.Skipped, tt.output.Skipped)
			case got.Time != tt.output.Time:
				t.Errorf("unexpected Time got: %+v, want: %+v", got.Time, tt.output.Time)
			case got.SystemOut != tt.output.SystemOut:
				t.Errorf("unexpected SystemOut got: %+v, want: %+v", got.SystemOut, tt.output.SystemOut)
			case got.SystemErr != tt.output.SystemErr:
				t.Errorf("unexpected SystemErr got: %+v, want: %+v", got.SystemErr, tt.output.SystemErr)
			case len(got.Testcases) != tt.output.Tests:
				t.Errorf("unexpected test length got: %+v, want: %+v", got.Tests, tt.output.Tests)
			}
		})
	}
}

func TestConvertToTestCase(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output []junit.Testcase
	}{
		{
			name:  "parse_passing_results",
			input: testPass,
			output: []junit.Testcase{
				{Name: "TestUpdateNSSwitchConfig", Time: "0.010"},
				{Name: "TestUpdateSSHConfig", Time: "0.020"},
				{Name: "TestUpdatePAMsshd", Time: "0.000"},
				{Name: "TestUpdateGroupConf", Time: "0.000"}},
		},
		{
			name:  "parse_failing_results",
			input: testFail,
			output: []junit.Testcase{
				{Time: "0.000", Name: "TestAlwaysFails", Failure: &junit.Result{
					Data: "    main_test.go:47: failed, message: heh\n    main_test.go:47: failed, message: heh2\n    main_test.go:47: failed, message: heh again"},
				},
				{Time: "0.000", Name: "TestUpdateNSSwitchConfig"},
				{Time: "0.000", Name: "TestUpdateSSHConfig"},
				{Time: "0.000", Name: "TestUpdatePAMsshd"},
				{Time: "0.000", Name: "TestUpdateGroupConf"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tcs, err := convertToTestCase(tt.input)
			if err != nil {
				t.Fatalf("unexpected error parsing: %v", err)
			}
			if len(tcs) != len(tt.output) {
				t.Errorf("unexpected expected: %v got: %v", tt.output, tcs)
			}
			for i := 0; i < len(tt.output); i++ {
				switch {
				case tcs[i].Classname != tt.output[i].Classname:
					t.Errorf("unexpected mismatched Classname got: %v but want: %v", tcs[i].Classname, tt.output[i].Classname)
				case tcs[i].Name != tt.output[i].Name:
					t.Errorf("unexpected mismatched Name got: %v but want: %v", tcs[i].Name, tt.output[i].Name)
				case tcs[i].Time != tt.output[i].Time:
					t.Errorf("unexpected mismatched Time got: %v but want: %v", tcs[i].Time, tt.output[i].Time)
				case tcs[i].Skipped != tt.output[i].Skipped:
					t.Errorf("unexpected mismatched Skipped got: %v but want: %v", tcs[i].Skipped, tt.output[i].Skipped)
				case (tcs[i].Failure != nil && tt.output[i].Failure == nil) || (tcs[i].Failure == nil && tt.output[i].Failure != nil):
					t.Errorf("unexpected mismatched Failure status got: %v but want: %v", tcs[i].Failure, tt.output[i].Failure)
				case tcs[i].Failure != nil && tcs[i].Failure.Data != tt.output[i].Failure.Data:
					t.Errorf("unexpected mismatched Failure Data got: %v but want: %v", tcs[i].Failure.Data, tt.output[i].Failure.Data)
				case tcs[i].SystemOut != tt.output[i].SystemOut:
					t.Errorf("unexpected mismatched SystemOut got: %v but want: %v", tcs[i].SystemOut, tt.output[i].SystemOut)
				}
			}
		})
	}
}
