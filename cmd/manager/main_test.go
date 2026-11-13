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

package main

import (
	"path/filepath"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	dstDir := t.TempDir()

	tests := []struct {
		name     string
		dstDir   string
		objName  string
		wantPath string
		wantErr  bool
	}{
		{
			name:     "normal_nested_object",
			objName:  "workflow/logs/serial.log",
			wantPath: filepath.Join(dstDir, "workflow/logs/serial.log"),
		},
		{
			name:     "single_file",
			objName:  "serial.log",
			wantPath: filepath.Join(dstDir, "serial.log"),
		},
		{
			name:     "leading_dotdot_in_segment_name",
			objName:  "..serial/log.txt",
			wantPath: filepath.Join(dstDir, "..serial/log.txt"),
		},
		{
			name:     "internal_dotdot_stays_within_dst",
			objName:  "workflow/../other/serial.log",
			wantPath: filepath.Join(dstDir, "other/serial.log"),
		},
		{
			name:     "relative_dst_nested",
			dstDir:   "artifacts",
			objName:  "workflow/serial.log",
			wantPath: filepath.Join("artifacts", "workflow/serial.log"),
		},
		{
			name:    "parent_escape",
			objName: "../escape.txt",
			wantErr: true,
		},
		{
			name:    "sibling_prefix_escape",
			objName: "../" + filepath.Base(dstDir) + "x/escape.txt",
			wantErr: true,
		},
		{
			name:    "relative_dst_escape",
			dstDir:  "artifacts",
			objName: "../escape.txt",
			wantErr: true,
		},
		{
			name:    "relative_dst_sibling_prefix_escape",
			dstDir:  "artifacts",
			objName: "../artifactsx/escape.txt",
			wantErr: true,
		},
		{
			name:    "dot_dst_escape",
			dstDir:  ".",
			objName: "../escape.txt",
			wantErr: true,
		},
		{
			name:    "dotdot_only",
			objName: "..",
			wantErr: true,
		},
		{
			name:    "deep_escape_after_prefix",
			objName: "workflow/logs/../../../escape.txt",
			wantErr: true,
		},
		{
			name:    "absolute_path",
			objName: "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "empty_path",
			objName: "",
			wantErr: true,
		},
		{
			name:    "dot_path",
			objName: ".",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := dstDir
			if tc.dstDir != "" {
				dir = tc.dstDir
			}
			got, err := safeJoin(dir, tc.objName)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("safeJoin(%q, %q) = %q, nil; want error", dir, tc.objName, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("safeJoin(%q, %q) returned unexpected error: %v", dir, tc.objName, err)
			}
			if got != tc.wantPath {
				t.Fatalf("safeJoin(%q, %q) = %q, want %q", dir, tc.objName, got, tc.wantPath)
			}
		})
	}
}
