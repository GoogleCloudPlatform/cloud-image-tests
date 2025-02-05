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

package pluginmanager

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

// Plugin struct represents the bare minimum plugin information required to
// create a test plugin.
type Plugin struct {
	PluginType  PluginType
	Name        string
	Revision    string
	Address     string
	InstallPath string
	EntryPath   string
	Protocol    string
	Manifest    *Manifest
	RuntimeInfo *RuntimeInfo
}

type RuntimeInfo struct {
	Pid int
}

type Manifest struct {
	StartAttempts int
	StopTimeout   time.Duration
	StartTimeout  time.Duration
}

type PluginType int

const (
	PluginTypeCore PluginType = iota
	PluginTypeDynamic

	linuxBaseDir      = "/var/lib/google-guest-agent"
	linuxConfigFile   = "/etc/default/instance_configs.cfg"
	linuxSocketDir    = "/run/google-guest-agent/plugin-connections"
	windowsBaseDir    = `C:\ProgramData\Google\Compute Engine\google-guest-agent`
	windowsConfigFile = `C:\Program Files\Google\Compute Engine\instance_configs.cfg`
	windowsSocketDir  = windowsBaseDir
)

func agentBaseDir(t *testing.T) string {
	t.Helper()

	id, err := utils.GetMetadata(utils.Context(t), "instance", "id")
	if err != nil {
		t.Fatalf("Failed to get instance id from MDS: %v", err)
	}

	baseDir := linuxBaseDir
	if utils.IsWindows() {
		baseDir = windowsBaseDir
	}

	return filepath.Join(baseDir, id)
}

func createTestSocketfile(t *testing.T) string {
	t.Helper()

	connectionsDir := linuxSocketDir
	if utils.IsWindows() {
		connectionsDir = windowsSocketDir
	}

	if err := os.MkdirAll(connectionsDir, 0755); err != nil {
		t.Fatalf("Failed to create test plugin connections directory(%q): %v", connectionsDir, err)
	}

	socket := filepath.Join(connectionsDir, "testplugin_1.sock")

	if err := os.WriteFile(socket, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create test plugin socket file(%q): %v", socket, err)
	}

	return socket
}

func setupTestPluginInstallDir(t *testing.T, baseDir string) (string, string) {
	t.Helper()

	testPluginInstallDir := filepath.Join(baseDir, "plugins", "testplugin")
	if err := os.MkdirAll(testPluginInstallDir, 0755); err != nil {
		t.Fatalf("Failed to create test plugin install directory(%q): %v", testPluginInstallDir, err)
	}

	entryPoint := filepath.Join(testPluginInstallDir, "plugin_executable")
	if err := os.WriteFile(entryPoint, []byte{}, 0755); err != nil {
		t.Fatalf("Failed to create test plugin entry point file(%q): %v", entryPoint, err)
	}

	return testPluginInstallDir, entryPoint
}

func createTestPlugin(t *testing.T) *Plugin {
	baseDir := agentBaseDir(t)

	pluginInfoFile := filepath.Join(baseDir, "agent_state", "plugin_info", "testplugin.gob")

	testPluginInstallDir, execPath := setupTestPluginInstallDir(t, baseDir)
	testsocket := createTestSocketfile(t)

	testplugin := &Plugin{
		PluginType:  PluginTypeDynamic,
		Name:        "testplugin",
		Revision:    "1",
		Protocol:    "unix",
		Address:     testsocket,
		EntryPath:   execPath,
		InstallPath: testPluginInstallDir,
		Manifest:    &Manifest{StartAttempts: 3, StopTimeout: time.Second * 3, StartTimeout: time.Second * 3},
		RuntimeInfo: &RuntimeInfo{Pid: 0},
	}

	b := new(bytes.Buffer)
	if err := gob.NewEncoder(b).Encode(testplugin); err != nil {
		t.Fatalf("Failed to encode plugin info: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(pluginInfoFile), 0755); err != nil {
		t.Fatalf("Failed to create test plugin info directory(%q): %v", filepath.Dir(pluginInfoFile), err)
	}

	if err := os.WriteFile(pluginInfoFile, b.Bytes(), 0644); err != nil {
		t.Fatalf("Failed to write plugin info to %q: %v", pluginInfoFile, err)
	}

	return testplugin
}

func ggactlCleanupPath(t *testing.T) string {
	t.Helper()

	if utils.IsWindows() {
		execPath := `C:\Program Files\Google\Compute Engine\agent\ggactl_plugin_cleanup.exe`
		if !utils.Exists(execPath, utils.TypeFile) {
			t.Skipf("ggactl_plugin_cleanup executable not found at %q", execPath)
		}
		return execPath
	}

	// On Linux the binary must be found in the PATH.
	execPath, err := exec.LookPath("ggactl_plugin_cleanup")
	if err != nil {
		t.Skipf("Failed to find ggactl_plugin_cleanup executable: %v", err)
	}

	return execPath
}

func enableAgentDebugLogging(t *testing.T) {
	cfgFile := linuxConfigFile
	if utils.IsWindows() {
		cfgFile = windowsConfigFile
	}

	content := `
[Core]
log_level = 4
log_verbosity = 4
	`

	content = fmt.Sprintf("\n%s\n", content)

	f, err := os.OpenFile(cfgFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to open config file(%q): %v", cfgFile, err)
	}
	defer f.Close()

	if _, err := f.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write to config file(%q): %v", cfgFile, err)
	}
}

func TestPluginCleanup(t *testing.T) {
	execPath := ggactlCleanupPath(t)
	plugin := createTestPlugin(t)
	enableAgentDebugLogging(t)

	tests := []struct {
		name     string
		filepath string
		fileType utils.FileType
	}{
		{
			name:     "install_dir",
			filepath: plugin.InstallPath,
			fileType: utils.TypeDir,
		},
		{
			name:     "entry_point",
			filepath: plugin.EntryPath,
			fileType: utils.TypeFile,
		},
		{
			name:     "address",
			filepath: plugin.Address,
			fileType: utils.TypeFile,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !utils.Exists(test.filepath, test.fileType) {
				t.Fatalf("File %q does not exist for test plugin", test.filepath)
			}
		})
	}

	cmd := exec.CommandContext(utils.Context(t), execPath, "all")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run ggactl_plugin_cleanup: %v, \noutput: \n%s", err, string(out))
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if utils.Exists(test.filepath, test.fileType) {
				t.Fatalf("File %q still exist for test plugin after cleanup", test.filepath)
			}
		})
	}
}
