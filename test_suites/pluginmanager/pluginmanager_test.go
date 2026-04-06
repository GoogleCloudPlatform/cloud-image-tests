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
	"runtime"
	"strings"
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

func agentLocalDir(t *testing.T) string {
	if utils.IsWindows() {
		return `C:\Program Files\Google\Compute Engine\agent`
	}
	return "/usr/lib/google/guest_agent"
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

func timeNow(t *testing.T) string {
	t.Helper()
	return time.Now().Format(time.RFC3339)
}

func TestCorePluginStop(t *testing.T) {
	if utils.IsCoreDisabled() {
		t.Skip("Core functionality is disabled, skipping the test.")
	}

	// Determine which version of the agent we have.
	_, err := exec.LookPath("ggactl_plugin")
	isNewAgent := err == nil

	// Fallback for Windows where the agent dir might not be in PATH.
	if !isNewAgent && utils.IsWindows() {
		winPluginPath := `C:\Program Files\Google\Compute Engine\agent\ggactl_plugin.exe`
		if _, err := os.Stat(winPluginPath); err == nil {
			isNewAgent = true
		}
	}

	if !isNewAgent {
		t.Log("Architecture: Legacy (Monolithic).")
		t.Skip("Core plugin lifecycle tests are not applicable to the legacy agent.")
	}

	// For the new agent, the core plugin is a direct child process, so stopping
	// the manager should stop the core plugin.
	// For the old agent, the core plugin is a detached process, so stopping the
	// manager should not stop the core plugin.

	// At this point, we know we are on the New Agent architecture.
	t.Log("Architecture: New (Plugin-based).")

	serviceToStop := "google-guest-agent-manager"
	if utils.IsWindows() {
		serviceToStop = "GCEAgentManager"
	}

	processToCheck := getCorePluginProcessName(t)

	// Use the runtime check to see if the plugin is actually a child or detached.
	isDetached := checkIsDetached(t, serviceToStop, processToCheck)
	t.Logf("Detached state detected as: %t", isDetached)

	stopAgentManager(t)
	verifyProcessState(t, processToCheck, isDetached)

}

// checkIsDetached determines if the plugin is a child of the manager.
// If it's not a child (e.g. PPID is 1), then it's detached.
func checkIsDetached(t *testing.T, managerService, pluginPath string) bool {
	var managerPid, pluginPpid string
	var err error

	if utils.IsWindows() {
		managerPid, err = getServicePidWindows(managerService)
		if err == nil {
			pluginPpid, err = getProcessPpidWindows(pluginPath)
		}
	} else {
		managerPid, err = getServicePidLinux(managerService)
		if err == nil {
			pluginPpid, err = getProcessPpidLinux(pluginPath)
		}
	}

	if err != nil {
		t.Logf("PPID detection failed: %v. Defaulting to architecture expectation.", err)
		return false
	}
	// If the plugin's PPID is not the manager's PID, then it's detached (returns true).
	return managerPid != pluginPpid
}

// getServicePidLinux fetches the MainPID of a systemd service.
func getServicePidLinux(serviceName string) (string, error) {
	// "--value" flag is not supported by systemd < 230 (like CentOS 7)
	cmd := exec.Command("systemctl", "show", serviceName, "--property=MainPID")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %q: %v, output: %s", cmd.String(), err, string(out))
	}

	// The output will be "MainPID=1234" or just "MainPID=" if not running
	output := strings.TrimSpace(string(out))
	parts := strings.Split(output, "=")
	if len(parts) != 2 || parts[1] == "" || parts[1] == "0" {
		return "", fmt.Errorf("service %s is not running (MainPID not found), logic check failed for parts: %q", serviceName, parts)
	}

	return parts[1], nil
}

// getProcessPpidLinux fetches the PPID of a process.
func getProcessPpidLinux(processPath string) (string, error) {
	// Get PID of the process first
	cmd := exec.Command("pgrep", "-f", processPath)
	pidOut, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %q: %v, output: %s", cmd.String(), err, string(pidOut))
	}
	pid := strings.TrimSpace(strings.Split(string(pidOut), "\n")[0])

	// Get PPID of that PID
	cmd = exec.Command("ps", "-o", "ppid=", "-p", pid)
	ppidOut, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to run %q: %v, output: %s", cmd.String(), err, string(ppidOut))
	}
	return strings.TrimSpace(string(ppidOut)), nil
}

// getServicePidWindows fetches the PID of a service.
func getServicePidWindows(serviceName string) (string, error) {
	cmd := fmt.Sprintf("(Get-Service %s).Id", serviceName)
	out, err := utils.RunPowershellCmd(cmd)
	return strings.TrimSpace(out.Stdout), err
}

// getProcessPpidWindows fetches the PPID of a process.
func getProcessPpidWindows(processPath string) (string, error) {
	var filter string

	// If the string is a path (contains backslashes)
	if strings.Contains(processPath, `\`) {
		// Escape backslashes for WMI/CIM
		escapedPath := strings.ReplaceAll(processPath, `\`, `\\`)
		filter = fmt.Sprintf("ExecutablePath='%s'", escapedPath)
	} else {
		// If it's just a name, ensure it has the .exe suffix
		name := processPath
		if !strings.HasSuffix(strings.ToLower(name), ".exe") {
			name += ".exe"
		}
		filter = fmt.Sprintf("Name='%s'", name)
	}

	// Wrap the filter in double quotes and the property in single quotes for PowerShell
	cmd := fmt.Sprintf(`(Get-CimInstance Win32_Process -Filter "%s").ParentProcessId`, filter)
	out, err := utils.RunPowershellCmd(cmd)

	// If no output is found, the process might not be running yet; return error to trigger fallback
	result := strings.TrimSpace(out.Stdout)
	if err != nil || result == "" {
		return "", fmt.Errorf("could not find PPID for %s: %v, \nstderr: \n%s, \nstdout: \n%s",
			processPath, err, out.Stderr, out.Stdout)
	}
	return result, nil
}

// verifyProcessState verifies that the process is in the expected state.
func verifyProcessState(t *testing.T, processPath string, expectRunning bool) {
	t.Helper()
	t.Logf("%v: Verifying %s (Expect Running: %t)", timeNow(t), processPath, expectRunning)

	var passed bool
	var lastOutput string

	for i := 0; i < 10; i++ {
		var found bool
		var currentOutput string

		if utils.IsWindows() {
			procName := filepath.Base(processPath)
			procName = strings.TrimSuffix(procName, ".exe")

			cmd := fmt.Sprintf(`Get-Process -Name "%s" -ErrorAction SilentlyContinue`, procName)
			status, _ := utils.RunPowershellCmd(cmd)

			currentOutput = status.Stdout + status.Stderr
			found = (strings.TrimSpace(status.Stdout) != "")
		} else {
			out, exists, err := utils.ProcessExistsLinux(processPath)
			if err != nil {
				t.Logf("Error checking process: %v", err)
				time.Sleep(3 * time.Second)
				continue
			}
			currentOutput = out
			found = exists
		}

		lastOutput = currentOutput
		if found == expectRunning {
			passed = true
			break
		}
		time.Sleep(3 * time.Second)
	}

	if !passed {
		expectedStr := "STOPPED"
		if expectRunning {
			expectedStr = "RUNNING (Detached)"
		}
		t.Errorf("Process mismatch for %q. Expected: %s", processPath, expectedStr)
		t.Fatalf("Last check output: \n%s", lastOutput)
	}
}

func writeConfigFile(conf string) error {
	file := "/etc/default/instance_configs.cfg"
	if utils.IsWindows() {
		file = `C:\Program Files\Google\Compute Engine\instance_configs.cfg`
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open instance_configs.cfg: %v", err)
	}
	if _, err := f.WriteString(conf); err != nil {
		return fmt.Errorf("Failed to write to instance_configs.cfg: %v", err)
	}
	return f.Close()
}

func disableACS(t *testing.T) {
	t.Helper()
	conf := `
[Core]
acs_client = false
`

	if err := writeConfigFile(conf); err != nil {
		t.Fatalf("Failed to disable ACS: %v", err)
	}
}

func enableLocalPlugin(t *testing.T, enable bool) {
	t.Helper()
	conf := fmt.Sprintf(`
[Core]
enable_local_plugins = %t`, enable)

	if err := writeConfigFile(conf); err != nil {
		t.Fatalf("Failed to enable local plugin: %v", err)
	}
}

func stopAgentManager(t *testing.T) {
	t.Helper()
	t.Logf("Stopping agent manager")
	if utils.IsWindows() {
		out, err := utils.RunPowershellCmd(`Stop-Service -Name GCEAgentManager -Verbose`)
		if err != nil {
			t.Fatalf("Failed to stop GCEAgentManager service: %v, \noutput: \n%+v", err, out)
		}
		return
	}
	cmd := exec.Command("systemctl", "stop", "google-guest-agent-manager")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to stop google-guest-agent-manager: %v, \noutput: \n%s", err, string(out))
	}
}

func startAgentManager(t *testing.T) {
	t.Helper()
	t.Logf("Starting agent manager")
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

// getCorePluginProcessName returns the process name of the core plugin based
// on the OS.
func getCorePluginProcessName(t *testing.T) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		return "CorePlugin"
	}
	_, err := os.Stat("/usr/lib/google/guest_agent/core_plugin")
	if err == nil {
		return "/usr/lib/google/guest_agent/core_plugin"
	}

	_, err = os.Stat("/usr/lib/google/guest_agent/GuestAgentCorePlugin/core_plugin")
	if err != nil {
		t.Fatalf("No core plugin paths found!")
	}
	return "/usr/lib/google/guest_agent/GuestAgentCorePlugin/core_plugin"
}

func verifyCorePluginExists(t *testing.T, shouldExist bool) {
	t.Helper()

	if utils.IsWindows() {
		utils.VerifyProcessExistsWindows(t, shouldExist, "CorePlugin")
	} else {
		_, foundOld, err := utils.ProcessExistsLinux("/usr/lib/google/guest_agent/core_plugin")
		if !foundOld || err != nil {
			t.Logf("Core plugin not found at /usr/lib/google/guest_agent/core_plugin, checking /usr/lib/google/guest_agent/GuestAgentCorePlugin/core_plugin")
			// Check for the new location of the core plugin.
			_, foundNew, err := utils.ProcessExistsLinux("/usr/lib/google/guest_agent/GuestAgentCorePlugin/core_plugin")
			if err != nil {
				t.Fatalf("Failed to check if core plugin exists: %v", err)
				return
			}

			found := foundOld || foundNew
			if found != shouldExist {
				foundStr := "not found"
				if found {
					foundStr = "found"
				}

				shouldExistStr := "not exist"
				if shouldExist {
					shouldExistStr = "exist"
				}

				pathString := "/usr/lib/google/guest_agent/core_plugin"
				if foundNew {
					pathString = "/usr/lib/google/guest_agent/GuestAgentCorePlugin/core_plugin"
				}

				if found {
					t.Fatalf("Core plugin process %s at %q, but should %s", foundStr, pathString, shouldExistStr)
				}
			}
		}
	}
}

func TestACSDisabled(t *testing.T) {
	// Skip if ggactl_plugin is not available to avoid running on images where
	// the new agent is not yet installed.
	ggactlCleanupPath(t)

	t.Logf("ACS Enabled")
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
	t.Logf("ACS Disabled")
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
	t.Logf("Should exist: %t", shouldExist)
	verifyCorePluginExists(t, shouldExist)
}

func TestLocalPlugin(t *testing.T) {
	ggactlCleanupPath(t)

	// Check for local plugins.
	pluginsDir := agentLocalDir(t)
	files, err := os.ReadDir(pluginsDir)
	if err != nil {
		t.Fatalf("Failed to read local plugins directory(%q): %v", pluginsDir, err)
	}
	var found bool
	var plugins []string
	for _, file := range files {
		if file.IsDir() {
			t.Logf("Found plugin: %s", file.Name())
			// Look for the manifest files. This is an indicator that the package
			// supports dynamically launching local plugins.
			if _, err := os.Stat(filepath.Join(pluginsDir, file.Name(), "manifest.binpb")); err == nil {
				found = true
				// Determine the binary name.
				pluginDirFiles, err := os.ReadDir(filepath.Join(pluginsDir, file.Name()))
				if err != nil {
					t.Fatalf("Failed to read local plugins directory(%q): %v", filepath.Join(pluginsDir, file.Name()), err)
				}
				for _, pluginDirFile := range pluginDirFiles {
					if pluginDirFile.Name() != "manifest.binpb" {
						t.Logf("Found potential executable: %s", pluginDirFile.Name())
						plugins = append(plugins, filepath.Join(file.Name(), pluginDirFile.Name()))
					}
				}
			}
		}
	}
	if !found {
		t.Fatalf("No local plugins found, at least core plugin should be found.")
	}

	// With local plugins enabled, check to see that all plugins are running.
	for _, plugin := range plugins {
		if runtime.GOOS == "windows" {
			// Remove the .exe extension from the plugin name for the verifications.
			plugin = strings.TrimSuffix(plugin, ".exe")
			utils.VerifyProcessExistsWindows(t, true, filepath.Base(plugin))
		} else {
			utils.VerifyProcessExistsLinux(t, true, filepath.Join(pluginsDir, plugin))
		}
	}

	// TODO(b/489540405): Add test to disable local plugins and verify that all
	// non-core plugins are not running.
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
