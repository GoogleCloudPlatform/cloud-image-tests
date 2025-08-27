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
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"google.golang.org/grpc"
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
		execPath := `C:\Program Files\Google\Compute Engine\agent\ggactl_plugin.exe`
		if !utils.Exists(execPath, utils.TypeFile) {
			t.Skipf("ggactl_plugin executable not found at %q", execPath)
		}
		return execPath
	}

	// On Linux the binary must be found in the PATH.
	execPath, err := exec.LookPath("ggactl_plugin")
	if err != nil {
		t.Skipf("Failed to find ggactl_plugin executable: %v", err)
	}

	return execPath
}

func TestPluginCleanup(t *testing.T) {
	execPath := ggactlCleanupPath(t)
	plugin := createTestPlugin(t)

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

	cmd := exec.CommandContext(utils.Context(t), execPath, "dynamic-cleanup")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run ggactl_plugin: %v, \noutput: \n%s", err, string(out))
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if utils.Exists(test.filepath, test.fileType) {
				t.Fatalf("File %q still exist for test plugin after cleanup", test.filepath)
			}
		})
	}
}

func stopPluginManager(t *testing.T) {
	t.Helper()
	if utils.IsWindows() {
		if err := utils.CheckPowershellSuccess("Stop-Service -Name GCEAgentManager -Verbose"); err != nil {
			t.Fatalf("Failed to stop GCEWindowsAgentManager service: %v", err)
		}
	} else {
		cmd := exec.Command("systemctl", "stop", "google-guest-agent-manager")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Failed to stop google-guest-agent service: %v, \noutput: \n%s", err, string(out))
		}
	}
}

func TestCorePluginStop(t *testing.T) {
	// Skip if ggactl_plugin is not available.
	path := ggactlCleanupPath(t)
	if utils.IsCoreDisabled() {
		t.Skip("Core plugin is disabled, skipping the test.")
	}
	stopPluginManager(t)
	command := exec.Command(path, "coreplugin", "stop")
	// Core plugin should be running even after the manager is stopped.
	if utils.IsWindows() {
		utils.ProcessExistsWindows(t, true, "CorePlugin")
	} else {
		utils.ProcessExistsLinux(t, true, "/usr/lib/google/guest_agent/core_plugin")
	}

	// ggactl command to stop the core plugin.
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to run ggactl_plugin: %v, \noutput: \n%s", err, string(out))
	}

	// Core plugin should be stopped.
	if utils.IsWindows() {
		utils.ProcessExistsWindows(t, false, "CorePlugin")
	} else {
		utils.ProcessExistsLinux(t, false, "/usr/lib/google/guest_agent/core_plugin")
	}
}

func disableACS(t *testing.T) {
	t.Helper()
	conf := `
[Core]
acs_client = false
`
	file := "/etc/default/instance_configs.cfg"
	if utils.IsWindows() {
		file = `C:\Program Files\Google\Compute Engine\instance_configs.cfg`
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("Failed to open instance_configs.cfg: %v", err)
	}
	if _, err := f.WriteString(conf); err != nil {
		t.Fatalf("Failed to write to instance_configs.cfg: %v", err)
	}
	f.Close()
}

func stopAgentManager(t *testing.T) {
	t.Helper()
	if utils.IsWindows() {
		out, err := utils.RunPowershellCmd(`Stop-Service -Name GCEAgentManager -Verbose`)
		if err != nil {
			t.Fatalf("Failed to restart GCEAgentManager service: %v, \noutput: \n%+v", err, out)
		}
		return
	}
	cmd := exec.Command("systemctl", "stop", "google-guest-agent-manager")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to restart google-guest-agent-manager: %v, \noutput: \n%s", err, string(out))
	}
}

func startAgentManager(t *testing.T) {
	t.Helper()
	if utils.IsWindows() {
		out, err := utils.RunPowershellCmd(`Start-Service -Name GCEAgentManager -Verbose`)
		if err != nil {
			t.Fatalf("Failed to start GCEAgentManager service: %v, \noutput: \n%+v", err, out)
		}
		return
	}
	cmd := exec.Command("systemctl", "start", "google-guest-agent-manager")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to start google-guest-agent-manager: %v, \noutput: \n%s", err, string(out))
	}
}

func configureTestACS(t *testing.T, socketPath string) {
	t.Helper()

	conf := `
[ACS]
host = %s
`
	file := "/etc/default/instance_configs.cfg"
	if utils.IsWindows() {
		file = `C:\Program Files\Google\Compute Engine\instance_configs.cfg`
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("Failed to open instance_configs.cfg: %v", err)
	}
	if _, err := f.WriteString(fmt.Sprintf(conf, socketPath)); err != nil {
		t.Fatalf("Failed to write to instance_configs.cfg: %v", err)
	}
	f.Close()
}

func TestACSDisabled(t *testing.T) {
	// Skip if ggactl_plugin is not available to avoid running on images where
	// the new agent is not yet installed.
	ggactlCleanupPath(t)

	stopAgentManager(t)
	server, socketPath := startTestServer(t)
	configureTestACS(t, socketPath)
	startAgentManager(t)
	// Wait for some activity on the ACS server.
	time.Sleep(time.Second * 60)
	server.Stop()
	if requestCount <= 0 {
		t.Errorf("Request count on ACS server: %d, want > 0 when ACS is enabled", requestCount)
	}

	// Reset the request count and disable ACS.
	requestCount = 0
	stopAgentManager(t)
	disableACS(t)
	server, socketPath = startTestServer(t)
	configureTestACS(t, socketPath)
	startAgentManager(t)
	// Wait for some activity on the ACS server.
	time.Sleep(time.Second * 60)
	server.Stop()
	if requestCount > 0 {
		t.Errorf("Request count on ACS server: %d, want 0 after ACS is disabled", requestCount)
	}

	// Core plugin should be running regardless of ACS being enabled or disabled.
	shouldExist := !utils.IsCoreDisabled()
	if utils.IsWindows() {
		utils.ProcessExistsWindows(t, shouldExist, "CorePlugin")
	} else {
		utils.ProcessExistsLinux(t, shouldExist, "/usr/lib/google/guest_agent/core_plugin")
	}
}

func startTestServer(t *testing.T) (*grpc.Server, string) {
	t.Helper()

	socketPath := filepath.Join(t.TempDir(), "test.sock")
	lis, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Skipf("Failed to listen on socket: %v, skipping the test as UDS is unavailable", err)
	}

	s := grpc.NewServer(grpc.UnknownServiceHandler(fakeServiceHandler))

	go func() {
		t.Logf("Serving gRPC server on UDS socket: %s", socketPath)
		if err := s.Serve(lis); err != nil {
			t.Logf("Failed to serve gRPC server: %v", err)
		}
		t.Logf("gRPC server stopped")
	}()
	return s, socketPath
}

func fakeServiceHandler(srv any, stream grpc.ServerStream) error {
	atomic.AddInt64(&requestCount, 1)
	return nil
}

// Request counter keeps track of the number of requests received by the fake
// gRPC server.
var requestCount int64
