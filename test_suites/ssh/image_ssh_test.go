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
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"golang.org/x/crypto/ssh"
)

func TestEmptyTest(t *testing.T) {
	_, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "ssh-keys")
	if err != nil {
		t.Fatalf("couldn't get ssh public key from metadata")
	}
	t.Logf("ssh target boot succesfully at %d", time.Now().UnixNano())
	time.Sleep(60 * time.Second)
}

// TestSSHInstanceKey test SSH completes successfully for an instance metadata key.
func TestSSHInstanceKey(t *testing.T) {
	// TODO(b/432559183): Re-enable this test once the bug is fixed.
	if utils.IsWindows() {
		t.Skip("Skipping test for windows due to b/432559183")
	}
	vmname, err := utils.GetRealVMName("server")
	if err != nil {
		t.Fatalf("failed to get real vm name: %v", err)
	}
	pembytes, err := utils.DownloadPrivateKey(utils.Context(t), user)
	if err != nil {
		t.Fatalf("failed to download private key: %v", err)
	}
	time.Sleep(60 * time.Second)
	t.Logf("connect to remote host at %d", time.Now().UnixNano())
	client, err := utils.CreateClient(user, fmt.Sprintf("%s:22", vmname), pembytes)
	if err != nil {
		t.Fatalf("user %s failed ssh to target host, %s, err %v", user, vmname, err)
	}
	if err := checkLocalUser(client, user); err != nil {
		t.Fatalf("failed to check local user: %v", err)
	}

	if err := checkSudoGroup(client, user); err != nil {
		t.Fatalf("failed to check sudo group: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Logf("failed to close client: %v", err)
	}
}

func TestSwitchDefaultConfig(t *testing.T) {
	_, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "ssh-keys")
	if err != nil {
		t.Fatalf("couldn't get ssh public key from metadata")
	}
	t.Logf("ssh target boot succesfully at %d", time.Now().UnixNano())
	cfg, err := os.ReadFile("/etc/default/instance_configs.cfg")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed to read instance configs: %v", err)
	}
	currentCfg := string(cfg)
	var newCfg string
	if strings.Contains(currentCfg, "deprovision_remove = false") {
		newCfg = strings.Replace(string(cfg), "deprovision_remove = false", "deprovision_remove = true", 1)
	} else {
		newCfg = fmt.Sprintf("%s\n[Accounts]\ndeprovision_remove = true", currentCfg)
	}
	if err := os.WriteFile("/etc/default/instance_configs.cfg", []byte(newCfg), 0644); err != nil {
		t.Fatalf("failed to write instance configs: %v", err)
	}
	utils.RestartAgent(utils.Context(t))
	time.Sleep(60 * time.Second)
}

func TestDeleteLocalUser(t *testing.T) {
	vmname, err := utils.GetRealVMName("server2")
	if err != nil {
		t.Fatalf("failed to get real vm name: %v", err)
	}
	pembytes, err := utils.DownloadPrivateKey(utils.Context(t), user)
	if err != nil {
		t.Fatalf("failed to download private key: %v", err)
	}

	time.Sleep(60 * time.Second)
	t.Logf("connect to remote host at %d", time.Now().UnixNano())

	client, err := utils.CreateClient(user, fmt.Sprintf("%s:22", vmname), pembytes)
	if err != nil {
		t.Fatalf("user %s failed ssh to target host, %s, err %v", user, vmname, err)
	}
	t.Cleanup(func() {
		if client != nil {
			client.Close()
		}
	})

	if err := checkSudoGroup(client, user2); err != nil {
		t.Fatalf("failed to check local user %s: %v", user2, err)
	}
	metadata := utils.GetInstanceMetadata(t, vmname)

	// Remove the user2's public key from the ssh-keys metadata.
	for _, item := range metadata.Items {
		var updateKeys []string
		if item.Key == "ssh-keys" {
			splitKeys := strings.Split(*item.Value, "\n")
			for _, key := range splitKeys {
				if strings.Contains(key, user2) {
					continue
				}
				updateKeys = append(updateKeys, key)
			}
			res := strings.Join(updateKeys, "\n")
			item.Value = &res
		}
	}

	utils.SetInstanceMetadata(t, vmname, metadata)
	time.Sleep(60 * time.Second)

	if err := checkSudoGroup(client, user2); err == nil {
		t.Fatalf("user %s still exists in sudo group on target host, %s", user2, vmname)
	}
	if err := checkLocalUser(client, user2); err == nil {
		t.Fatalf("user %s still exists on target host, %s, err: %v", user2, vmname, err)
	}
}

func TestDeleteUserDefault(t *testing.T) {
	// TODO(b/432559183): Re-enable this test once the bug is fixed.
	if utils.IsWindows() {
		t.Skip("Skipping test for windows due to b/432559183")
	}
	vmname, err := utils.GetRealVMName("server")
	if err != nil {
		t.Fatalf("failed to get real vm name: %v", err)
	}
	pembytes, err := utils.DownloadPrivateKey(utils.Context(t), user2)
	if err != nil {
		t.Fatalf("failed to download private key: %v", err)
	}
	time.Sleep(60 * time.Second)
	t.Logf("connect to remote host at %d", time.Now().UnixNano())
	client, err := utils.CreateClient(user2, fmt.Sprintf("%s:22", vmname), pembytes)
	if err != nil {
		t.Fatalf("user %s failed ssh to target host, %s, err %v", user2, vmname, err)
	}
	t.Cleanup(func() {
		if client != nil {
			client.Close()
		}
	})
	if err := checkSudoGroup(client, user2); err != nil {
		t.Fatalf("failed to check local user %s: %v", user2, err)
	}
	metadata := utils.GetInstanceMetadata(t, vmname)

	// Remove the user2's public key from the ssh-keys metadata.
	for _, item := range metadata.Items {
		var updateKeys []string
		if item.Key == "ssh-keys" {
			splitKeys := strings.Split(*item.Value, "\n")
			for _, key := range splitKeys {
				if strings.Contains(key, user2) {
					continue
				}
				updateKeys = append(updateKeys, key)
			}
			res := strings.Join(updateKeys, "\n")
			item.Value = &res
		}
	}

	utils.SetInstanceMetadata(t, vmname, metadata)
	time.Sleep(60 * time.Second)

	client2, err := utils.CreateClient(user2, fmt.Sprintf("%s:22", vmname), pembytes)
	t.Cleanup(func() {
		if client2 != nil {
			client2.Close()
		}
	})
	if err == nil {
		t.Fatalf("user %s successfully ssh to target host, %s", user2, vmname)
	}

	if err := checkSudoGroup(client, user2); err == nil {
		t.Fatalf("user %s still exists in sudo group on target host, %s", user2, vmname)
	}
	// Default config is to keep the user in /etc/passwd
	if err := checkLocalUser(client, user2); err != nil {
		t.Fatalf("user %s does not exist on target host, %s, err: %v", user2, vmname, err)
	}
}

// checkLocalUser test that the user account exists in /etc/passwd on linux
// or in Get-LocalUser output on windows
func checkLocalUser(client *ssh.Client, user string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	var findUsercmd string
	if utils.IsWindows() {
		findUsercmd = fmt.Sprintf(`powershell.exe -NonInteractive -NoLogo -NoProfile "Get-LocalUser -Name %s"`, user)
	} else {
		findUsercmd = fmt.Sprintf("grep %s: /etc/passwd", user)
	}
	if err := session.Run(findUsercmd); err != nil {
		return err
	}
	return nil
}

// checkSudoGroup test that the user account exists in sudo group on linux
// administrator group on windows
func checkSudoGroup(client *ssh.Client, user string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	var findInGrpcmd string
	if utils.IsWindows() {
		findInGrpcmd = fmt.Sprintf(`powershell.exe -NonInteractive -NoLogo -NoProfile "Get-LocalGroupMember -Group Administrators | Where-Object Name -Match %s"`, user)
	} else {
		findInGrpcmd = fmt.Sprintf("grep 'google-sudoers:.*%s' /etc/group", user)
	}
	out, err := session.Output(findInGrpcmd)
	if err != nil {
		return fmt.Errorf("%s err: %v; stderr: %s", findInGrpcmd, err, session.Stderr)
	}
	if utils.IsWindows() && !strings.Contains(string(out), user) {
		// The command on windows will exit successfully even with no match
		return fmt.Errorf("could not find user in Administrators group")
	}
	return nil
}
