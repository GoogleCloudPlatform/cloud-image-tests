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
	"context"
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

// TestSSHChangeKey test that given a user and a new key, guest agent will
// update the user's key in $HOME/.ssh/authorized_keys.
//
// The test will first attempt a loopback connection with the user's old key and
// expect it to succeed, an attempt to connect with the new key should fail at
// this point.
//
// It will later change the user's key to the previously one created in setup.go
// and use the new key to attempt a connection, which should succeed. An attempt
// to connect with the old key should fail at this point.
func TestSSHChangeKey(t *testing.T) {
	vmname, err := utils.GetRealVMName(utils.Context(t), "server2")
	if err != nil {
		t.Fatalf("failed to get real vm name: %v", err)
	}

	// Private key for user3, this is the original key that will be used for the
	// initial test.
	user3PemBytes, err := utils.DownloadPrivateKey(utils.Context(t), user3)
	if err != nil {
		t.Fatalf("failed to download private key: %v", err)
	}

	// Private key for user4, this is the new key that will be used to change
	// user3 key to and test the connection.
	user4PemBytes, err := utils.DownloadPrivateKey(utils.Context(t), user4)
	if err != nil {
		t.Fatalf("failed to download private key: %v", err)
	}

	closeClient := func(c *ssh.Client) {
		if c != nil {
			c.Close()
		}
	}

	// Initial success case, with user3 and user3PemBytes.
	t.Logf("connect to remote host at %d, with valid key, should succeed", time.Now().UnixNano())
	c1, err := utils.CreateClient(user3, fmt.Sprintf("%s:22", vmname), user3PemBytes)
	if err != nil {
		t.Fatalf("user %s failed ssh to target host, %s, err %v", user3, vmname, err)
	}
	t.Cleanup(func() {
		closeClient(c1)
	})

	// Initial failure case, with user3 and user4PemBytes (we haven't changed the
	// user's key yet, it must fail).
	t.Logf("connect to remote host at %d, with invalid key, should fail", time.Now().UnixNano())
	c2, err := utils.CreateClient(user3, fmt.Sprintf("%s:22", vmname), user4PemBytes)
	if err == nil {
		t.Fatalf("user %s succeeded ssh to target host with invalid key, %s", user3, vmname)
	}
	t.Cleanup(func() {
		closeClient(c2)
	})

	// Change the user's key to the new key.
	newKey, err := utils.GetMetadata(utils.Context(t), "instance", "attributes", "target-public-key")
	if err != nil {
		t.Fatalf("couldn't get ssh target public key from metadata: %v", err)
	}

	metadata := utils.GetInstanceMetadata(t, vmname)

	for _, item := range metadata.Items {
		var updateKeys []string
		if item.Key == "ssh-keys" {
			splitKeys := strings.Split(*item.Value, "\n")
			for _, key := range splitKeys {
				if strings.Contains(key, user3) {
					key = fmt.Sprintf("%s:%s", user3, newKey)
				}
				updateKeys = append(updateKeys, key)
			}
			res := strings.Join(updateKeys, "\n")
			item.Value = &res
		}
	}

	utils.SetInstanceMetadata(t, vmname, metadata)

	// Wait for the metadata to be updated and guest-agent to actuate and update
	// the user's key.
	t.Logf("waiting for metadata to be updated and guest-agent to actuate and update the user's key")
	time.Sleep(60 * time.Second)

	// Success case, with user3 and user4PemBytes (it should succeed).
	t.Logf("connect to remote host at %d, with valid key, should succeed", time.Now().UnixNano())
	c3, err := utils.CreateClient(user3, fmt.Sprintf("%s:22", vmname), user4PemBytes)
	if err != nil {
		t.Fatalf("user %s failed ssh to target host with new key, %s, err %v", user3, vmname, err)
	}
	t.Cleanup(func() {
		closeClient(c3)
	})

	// Failure case, with user3 and user3PemBytes (it should fail).
	t.Logf("connect to remote host at %d, with invalid key, should fail", time.Now().UnixNano())
	c4, err := utils.CreateClient(user3, fmt.Sprintf("%s:22", vmname), user3PemBytes)
	if err == nil {
		t.Fatalf("user %s succeeded ssh to target host with invalid key, %s", user3, vmname)
	}
	t.Cleanup(func() {
		closeClient(c4)
	})
}

// TestSSHInstanceKey test SSH completes successfully for an instance metadata key.
func TestSSHInstanceKey(t *testing.T) {
	vmname, err := utils.GetRealVMName(utils.Context(t), "server")
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

func switchDeprovisionRemove(t *testing.T) {
	t.Helper()
	cfg, err := os.ReadFile("/etc/default/instance_configs.cfg")
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed to read instance configs: %v", err)
	}
	currentCfg := string(cfg)
	t.Cleanup(func() {
		if err := os.WriteFile("/etc/default/instance_configs.cfg", []byte(currentCfg), 0644); err != nil {
			t.Logf("failed to restore instance configs in cleanup: %v", err)
			return
		}
		if err := utils.RestartAgent(utils.Context(t)); err != nil {
			t.Logf("failed to restart agent in cleanup: %v", err)
		}
	})
	var newCfg string
	if strings.Contains(currentCfg, "deprovision_remove = false") {
		newCfg = strings.Replace(string(cfg), "deprovision_remove = false", "deprovision_remove = true", 1)
	} else {
		newCfg = fmt.Sprintf("%s\n[Accounts]\ndeprovision_remove = true", currentCfg)
	}
	if err := os.WriteFile("/etc/default/instance_configs.cfg", []byte(newCfg), 0644); err != nil {
		t.Fatalf("failed to write instance configs: %v", err)
	}
	if err := utils.RestartAgent(utils.Context(t)); err != nil {
		t.Fatalf("failed to restart agent: %v", err)
	}
}

func userExistsInPasswd(user string) (bool, error) {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, user+":") {
			return true, nil
		}
	}
	return false, nil
}

func userInSudoGroup(user string) (bool, error) {
	data, err := os.ReadFile("/etc/group")
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "google-sudoers:") {
			parts := strings.Split(line, ":")
			if len(parts) >= 4 {
				for _, u := range strings.Split(parts[3], ",") {
					if strings.TrimSpace(u) == user {
						return true, nil
					}
				}
			}
		}
	}
	return false, nil
}

func waitForUserState(t *testing.T, user string, wantInGroup, wantInPasswd bool, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(utils.Context(t), timeout)
	defer cancel()
	for {
		inGroup, err := userInSudoGroup(user)
		if err != nil {
			t.Fatalf("failed to check sudo group: %v", err)
		}
		inPasswd, err := userExistsInPasswd(user)
		if err != nil {
			t.Fatalf("failed to check passwd: %v", err)
		}
		if inGroup == wantInGroup && inPasswd == wantInPasswd {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for user %s state (want inGroup=%v, inPasswd=%v; got inGroup=%v, inPasswd=%v)",
				user, wantInGroup, wantInPasswd, inGroup, inPasswd)
		case <-time.After(1 * time.Second):
		}
	}
}

func removeUserSSHKey(t *testing.T, vmname, user string) {
	t.Helper()
	metadata := utils.GetInstanceMetadata(t, vmname)
	var foundKey, foundUser bool
	for _, item := range metadata.Items {
		if item.Key == "ssh-keys" {
			foundKey = true
			var updateKeys []string
			splitKeys := strings.Split(*item.Value, "\n")
			for _, key := range splitKeys {
				if strings.HasPrefix(key, user+":") {
					foundUser = true
					continue
				}
				updateKeys = append(updateKeys, key)
			}
			res := strings.Join(updateKeys, "\n")
			item.Value = &res
		}
	}
	if !foundKey {
		t.Fatalf("ssh-keys attribute not found in instance metadata for %s", vmname)
	}
	if !foundUser {
		t.Fatalf("user %s not found in ssh-keys metadata for %s", user, vmname)
	}
	utils.SetInstanceMetadata(t, vmname, metadata)
}

func TestDeleteLocalUser(t *testing.T) {
	vmname, err := utils.GetRealVMName(utils.Context(t), "server2")
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
	removeUserSSHKey(t, vmname, user2)
	time.Sleep(60 * time.Second)
	if err := checkSudoGroup(client, user2); err == nil {
		t.Fatalf("user %s still exists in sudo group on target host, %s", user2, vmname)
	}
	if err := checkLocalUser(client, user2); err == nil {
		t.Fatalf("user %s still exists on target host, %s, err: %v", user2, vmname, err)
	}
}

func TestDeleteUserDefault(t *testing.T) {
	vmname, err := utils.GetRealVMName(utils.Context(t), "server")
	if err != nil {
		t.Fatalf("failed to get real vm name: %v", err)
	}
	t.Logf("vmname: %s", vmname)
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
	removeUserSSHKey(t, vmname, user2)
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
