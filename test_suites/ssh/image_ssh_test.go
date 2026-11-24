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

const (
	pollInterval        = 2 * time.Second
	connectTimeout      = 3 * time.Minute
	actuationTimeout    = 2 * time.Minute
	instanceConfigsPath = "/etc/default/instance_configs.cfg"
)

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
	ctx := utils.Context(t)
	vmname, err := utils.GetInstanceName(ctx)
	if err != nil {
		t.Fatalf("failed to get instance name: %v", err)
	}
	addr := fmt.Sprintf("%s:22", vmname)

	user3PemBytes, err := utils.DownloadPrivateKey(ctx, user3)
	if err != nil {
		t.Fatalf("failed to download private key: %v", err)
	}
	user4PemBytes, err := utils.DownloadPrivateKey(ctx, user4)
	if err != nil {
		t.Fatalf("failed to download private key: %v", err)
	}

	dialSSH(ctx, t, user3, addr, user3PemBytes)
	if c, err := utils.CreateClient(user3, addr, user4PemBytes); err == nil { // if NO error
		c.Close()
		t.Fatalf("user %s succeeded ssh to target host with invalid key, %s", user3, vmname)
	}

	newKey, err := utils.GetMetadata(ctx, "instance", "attributes", "target-public-key")
	if err != nil {
		t.Fatalf("couldn't get ssh target public key from metadata: %v", err)
	}
	updateUserSSHKey(t, vmname, user3, newKey)

	err = utils.PollUntil(ctx, "rotated SSH key actuation", pollInterval, actuationTimeout, func() (bool, string) {
		cNew, newErr := utils.CreateClient(user3, addr, user4PemBytes)
		if newErr != nil {
			return false, newErr.Error()
		}
		cNew.Close()
		if cOld, oldErr := utils.CreateClient(user3, addr, user3PemBytes); oldErr == nil { // if NO error
			cOld.Close()
			return false, "old key still accepted"
		}
		return true, ""
	})
	if err != nil {
		t.Fatalf("key rotation for %s on %s failed: %v", user3, vmname, err)
	}
}

// TestSSHInstanceKey test SSH completes successfully for an instance metadata key.
func TestSSHInstanceKey(t *testing.T) {
	ctx := utils.Context(t)
	vmname, err := utils.GetRealVMName(ctx, "server")
	if err != nil {
		t.Fatalf("failed to get real vm name: %v", err)
	}
	pembytes, err := utils.DownloadPrivateKey(ctx, user)
	if err != nil {
		t.Fatalf("failed to download private key: %v", err)
	}
	client := dialSSH(ctx, t, user, fmt.Sprintf("%s:22", vmname), pembytes)
	// Poll because the guest agent writes authorized_keys / creates the account
	// just before adding google-sudoers or Administrators membership.
	err = utils.PollUntil(ctx, user+" account and privilege group on "+vmname, pollInterval, actuationTimeout, func() (bool, string) {
		if userErr := checkLocalUser(client, user); userErr != nil {
			return false, userErr.Error()
		}
		if grpErr := checkSudoGroup(client, user); grpErr != nil {
			return false, grpErr.Error()
		}
		return true, ""
	})
	if err != nil {
		t.Fatalf("failed to verify %s on %s: %v", user, vmname, err)
	}
}

func TestDeleteLocalUser(t *testing.T) {
	utils.LinuxOnly(t)
	ctx := utils.Context(t)
	vmname, err := utils.GetInstanceName(ctx)
	if err != nil {
		t.Fatalf("failed to get instance name: %v", err)
	}
	pembytes, err := utils.DownloadPrivateKey(ctx, user)
	if err != nil {
		t.Fatalf("failed to download private key: %v", err)
	}
	// Connect as user (not user5) so no active login session blocks userdel -r on user5.
	client := dialSSH(ctx, t, user, fmt.Sprintf("%s:22", vmname), pembytes)
	err = utils.PollUntil(ctx, user5+" initial provisioning", pollInterval, actuationTimeout, func() (bool, string) {
		if err := checkSudoGroup(client, user5); err != nil {
			return false, err.Error()
		}
		return true, ""
	})
	if err != nil {
		t.Fatalf("failed to check local user %s: %v", user5, err)
	}

	enableDeprovisionRemove(ctx, t)
	updateUserSSHKey(t, vmname, user5, "")

	err = utils.PollUntil(ctx, user5+" account deletion", pollInterval, actuationTimeout, func() (bool, string) {
		if checkSudoGroup(client, user5) == nil { // if NO error
			return false, "still in google-sudoers"
		}
		if checkLocalUser(client, user5) == nil { // if NO error
			return false, "still in /etc/passwd"
		}
		return true, ""
	})
	if err != nil {
		t.Fatalf("user %s still exists on target host %s: %v", user5, vmname, err)
	}
}

func TestDeleteUserDefault(t *testing.T) {
	utils.LinuxOnly(t)
	ctx := utils.Context(t)
	vmname, err := utils.GetInstanceName(ctx)
	if err != nil {
		t.Fatalf("failed to get instance name: %v", err)
	}
	addr := fmt.Sprintf("%s:22", vmname)
	pembytes, err := utils.DownloadPrivateKey(ctx, user2)
	if err != nil {
		t.Fatalf("failed to download private key: %v", err)
	}
	client := dialSSH(ctx, t, user2, addr, pembytes)
	err = utils.PollUntil(ctx, user2+" initial provisioning", pollInterval, actuationTimeout, func() (bool, string) {
		if err := checkSudoGroup(client, user2); err != nil {
			return false, err.Error()
		}
		return true, ""
	})
	if err != nil {
		t.Fatalf("failed to check local user %s: %v", user2, err)
	}

	updateUserSSHKey(t, vmname, user2, "")

	err = utils.PollUntil(ctx, user2+" key and group revocation", pollInterval, actuationTimeout, func() (bool, string) {
		if checkSudoGroup(client, user2) == nil { // if NO error
			return false, "still in google-sudoers"
		}
		if c2, sshErr := utils.CreateClient(user2, addr, pembytes); sshErr == nil { // if NO error
			c2.Close()
			return false, "SSH with removed key still succeeded"
		}
		return true, ""
	})
	if err != nil {
		t.Fatalf("default deprovisioning for %s on %s failed: %v", user2, vmname, err)
	}
	// Default config is to keep the user in /etc/passwd
	if err := checkLocalUser(client, user2); err != nil {
		t.Fatalf("user %s does not exist on target host, %s, err: %v", user2, vmname, err)
	}
}

func enableDeprovisionRemove(ctx context.Context, t *testing.T) {
	t.Helper()
	origCfg, err := os.ReadFile(instanceConfigsPath)
	existed := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed to read instance configs: %v", err)
	}
	currentCfg := string(origCfg)
	var newCfg string
	if strings.Contains(currentCfg, "deprovision_remove = false") {
		newCfg = strings.Replace(currentCfg, "deprovision_remove = false", "deprovision_remove = true", 1)
	} else {
		newCfg = fmt.Sprintf("%s\n[Accounts]\ndeprovision_remove = true\n", currentCfg)
	}
	if err := os.WriteFile(instanceConfigsPath, []byte(newCfg), 0644); err != nil {
		t.Fatalf("failed to write instance configs: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if existed {
			if err := os.WriteFile(instanceConfigsPath, origCfg, 0644); err != nil {
				t.Logf("failed to restore instance configs: %v", err)
			}
		} else if err := os.Remove(instanceConfigsPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Logf("failed to remove instance configs: %v", err)
		}
		if err := utils.RestartAgent(cleanupCtx); err != nil {
			t.Logf("failed to restart agent during cleanup: %v", err)
		}
	})
	if err := utils.RestartAgent(ctx); err != nil {
		t.Fatalf("failed to restart agent after enabling deprovision_remove: %v", err)
	}
}

func dialSSH(ctx context.Context, t *testing.T, username, addr string, pemBytes []byte) *ssh.Client {
	t.Helper()
	var client *ssh.Client
	err := utils.PollUntil(ctx, "SSH connection for "+username+"@"+addr, pollInterval, connectTimeout, func() (bool, string) {
		c, dialErr := utils.CreateClient(username, addr, pemBytes)
		if dialErr != nil {
			return false, dialErr.Error()
		}
		client = c
		return true, ""
	})
	if err != nil {
		t.Fatalf("user %s failed ssh to %s: %v", username, addr, err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("failed to close client: %v", err)
		}
	})
	return client
}

// updateUserSSHKey replaces username's entry in ssh-keys metadata with
// newPublicKey, or removes username's entry when newPublicKey is empty.
func updateUserSSHKey(t *testing.T, vmname, username, newPublicKey string) {
	t.Helper()
	metadata := utils.GetInstanceMetadata(t, vmname)
	prefix := username + ":"
	found := false
	for _, item := range metadata.Items {
		if item == nil || item.Key != "ssh-keys" || item.Value == nil {
			continue
		}
		var updateKeys []string
		for _, key := range strings.Split(*item.Value, "\n") {
			if strings.HasPrefix(strings.TrimSpace(key), prefix) {
				found = true
				if newPublicKey == "" {
					continue
				}
				key = fmt.Sprintf("%s:%s", username, newPublicKey)
			}
			updateKeys = append(updateKeys, key)
		}
		res := strings.Join(updateKeys, "\n")
		item.Value = &res
	}
	if !found {
		t.Fatalf("user %s not found in ssh-keys metadata on %s", username, vmname)
	}
	utils.SetInstanceMetadata(t, vmname, metadata)
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
