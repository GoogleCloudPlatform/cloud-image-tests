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

package ssh

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"golang.org/x/crypto/ssh"
)

// TestMatchingKeysInGuestAttributes validate that host keys in guest attributes match those on disk.
func TestMatchingKeysInGuestAttributes(t *testing.T) {
	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatalf("couldn't get image from metadata")
	}
	if utils.IsCOS(image) {
		// COS is not a supported OS.
		t.Skip("COS is not a supported OS for storing hostkeys via guest attributes.")
	}
	diskEntries, err := utils.GetHostKeysFromDisk()
	if err != nil {
		t.Fatalf("failed to get host key from disk %v", err)
	}

	hostkeys, err := utils.GetMetadata(utils.Context(t), "instance", "guest-attributes", "hostkeys", "/")
	if err != nil {
		t.Fatal(err)

	}
	// validate that the guest agent copies the host keys from disk to the metadata.
	// https://github.com/GoogleCloudPlatform/guest-agent/blob/main/google_guest_agent/instance_setup.go
	for _, keyType := range strings.Split(hostkeys, "\n") {
		if keyType == "" {
			continue
		}
		keyValue, err := utils.GetMetadata(utils.Context(t), "instance", "guest-attributes", "hostkeys", keyType)
		if err != nil {
			t.Fatal(err)
		}
		valueFromDisk, found := diskEntries[keyType]
		if !found {
			t.Fatalf("failed finding key %s from disk", keyType)
		}
		if valueFromDisk != strings.TrimSpace(keyValue) {
			t.Fatalf("host keys %s %s in guest attributes not match those on disk %s", keyType, keyValue, valueFromDisk)
		}
	}
}

// TestHostKeysAreUnique validate that host keys from disk is unique between instances.
func TestHostKeysAreUnique(t *testing.T) {
	ctx := utils.Context(t)
	vmname, err := utils.GetRealVMName(ctx, "server")
	if err != nil {
		t.Fatalf("failed to get real vm name: %v", err)
	}
	pembytes, err := utils.DownloadPrivateKey(ctx, user)
	if err != nil {
		t.Fatalf("failed to download private key: %v", err)
	}
	var localDiskEntries map[string]string
	err = utils.PollUntil(ctx, "local host keys", pollInterval, actuationTimeout, func() (bool, string) {
		var localErr error
		localDiskEntries, localErr = utils.GetHostKeysFromDisk()
		if localErr != nil {
			return false, localErr.Error()
		}
		return true, ""
	})
	if err != nil {
		t.Fatalf("failed to get host key from disk: %v", err)
	}

	client := dialSSH(ctx, t, user, fmt.Sprintf("%s:22", vmname), pembytes)
	var remoteDiskEntries map[string]string
	err = utils.PollUntil(ctx, "remote host keys on "+vmname, pollInterval, actuationTimeout, func() (bool, string) {
		entries, remoteErr := getRemoteHostKeys(client)
		if remoteErr != nil {
			return false, remoteErr.Error()
		}
		for keyType := range localDiskEntries {
			if _, found := entries[keyType]; !found {
				return false, fmt.Sprintf("ssh key %s not found on remote disk entries", keyType)
			}
		}
		remoteDiskEntries = entries
		return true, ""
	})
	if err != nil {
		t.Fatalf("failed to get host key from remote, err: %v", err)
	}

	for keyType, localValue := range localDiskEntries {
		if localValue == remoteDiskEntries[keyType] {
			t.Fatal("host key value is not unique")
		}
	}
}

func getRemoteHostKeys(client *ssh.Client) (map[string]string, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to open ssh session: %s", err)
	}
	defer session.Close()
	cmd := "cat /etc/ssh/ssh_host_*_key.pub"
	if utils.IsWindows() {
		cmd = `powershell.exe -NonInteractive -NoLogo -NoProfile 'Get-Content C:\ProgramData\ssh\ssh_host_*_key.pub'`
	}
	bytes, err := session.Output(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to run %s in remote session: %s; stdout: %s; stderr: %s", cmd, err, bytes, session.Stderr)
	}
	hostKeyMap, err := utils.ParseHostKey(bytes)
	if err != nil {
		return nil, err
	}
	if utils.IsWindows() {
		// A session will only accept a single Output call, so we need a new one
		winrmsession, err := client.NewSession()
		if err != nil {
			return nil, fmt.Errorf("failed to open ssh session: %s", err)
		}
		defer winrmsession.Close()
		winrmThumb, err := winrmsession.Output(`powershell.exe -NonInteractive -NoLogo -NoProfile "Get-ChildItem Cert:\LocalMachine\My\ | Where-Object {$_.Subject -eq \"CN=$(hostname)\"} | Format-List -Property Thumbprint"`)
		if err != nil {
			return nil, fmt.Errorf("failed to get winrm key thumb remotely: %s", err)
		}
		winrm := strings.TrimPrefix(strings.TrimSpace(string(winrmThumb)), "Thumbprint : ")
		if winrm == "" {
			return nil, fmt.Errorf("Could not find winrm cert thumbprint, got %s from cert store query", winrm)
		}
		hostKeyMap["winrm"] = winrm
		rdpsession, err := client.NewSession()
		if err != nil {
			return nil, fmt.Errorf("failed to open ssh session: %s", err)
		}
		defer rdpsession.Close()
		rdpThumb, err := rdpsession.Output(`powershell.exe -NonInteractive -NoLogo -NoProfile "Get-ChildItem 'Cert:\LocalMachine\Remote Desktop\' | Where-Object {$_.Subject -eq \"CN=$(hostname)\"} | Format-List -Property Thumbprint"`)
		if err != nil {
			return nil, fmt.Errorf("failed to get rdp key thumb remotely: %s", err)
		}
		rdp := strings.TrimPrefix(strings.TrimSpace(string(rdpThumb)), "Thumbprint : ")
		if rdp == "" {
			return nil, fmt.Errorf("Could not find rdp cert thumbprint, got %s from cert store query", rdp)
		}
		hostKeyMap["rdp"] = rdp
	}
	return hostKeyMap, err
}

func TestHostKeysNotOverrideAfterAgentRestart(t *testing.T) {
	hostKeyBeforeRestart, err := utils.GetHostKeysFileFromDisk()
	if err != nil {
		t.Fatalf("failed to get host keys from disk %v", err)
	}
	if err := utils.RestartAgent(utils.Context(t)); err != nil {
		t.Fatal(err)
	}
	hostKeyAfterRestart, err := utils.GetHostKeysFileFromDisk()
	if err != nil {
		t.Fatalf("failed to get host key from disk %v", err)
	}
	if string(hostKeyBeforeRestart) != string(hostKeyAfterRestart) {
		t.Fatalf("host keys are changed after guest agent restart %v", err)
	}
}
