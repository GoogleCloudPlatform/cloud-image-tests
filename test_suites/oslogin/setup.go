// Copyright 2023 Google LLC
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

// Package oslogin is a CIT suite for testing oslogin ssh with and without 2fa.
// See the README.md file for required project setup to run this suite.
package oslogin

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"sync"

	"cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	"google.golang.org/api/iterator"
)

var (
	// Name is the name of the test package. It must match the directory name.
	Name = "oslogin"

	// initialize2FA makes sure we only initialize 2FA users list once.
	initialize2FA sync.Once
)

// test2FAUser encapsulates a test user for 2FA tests.
type test2FAUser struct {
	// email is the secret for the email of this test user.
	email string

	// twoFA is the secret for the 2FA secret of this test user.
	twoFA string

	// sshKey is the secret for the private SSH key of this test user.
	sshKey string
}

const (
	computeScope  = "https://www.googleapis.com/auth/compute"
	platformScope = "https://www.googleapis.com/auth/cloud-platform"

	// 2FA metadata keys.
	normal2FAUser   = "normal-2fa-user"
	normal2FAKey    = "normal-2fa-key"
	normal2FASSHKey = "normal-2fa-ssh-key"
	admin2FAUser    = "admin-2fa-user"
	admin2FAKey     = "admin-2fa-key"
	admin2FASSHKey  = "admin-2fa-ssh-key"
)

var (
	// counter keeps track of the number of OSLogin tests running.
	counter int

	// test2FAUsers is the list of 2FA test users to use for this test.
	twoFATestUsers = []test2FAUser{
		{
			email:  "normal-2fa-user",
			sshKey: "normal-2fa-ssh-key",
			twoFA:  "normal-2fa-key",
		},
	}

	// twoFAAdminTestUsers is the list of 2FA admin test users to use for this test.
	// Ideally there is one admin test user for every normal test user to form "pairs".
	twoFAAdminTestUsers = []test2FAUser{
		{
			email:  "admin-2fa-user",
			sshKey: "admin-2fa-ssh-key",
			twoFA:  "admin-2fa-key",
		},
	}
)

// TestSetup sets up the test workflow.
func TestSetup(t *imagetest.TestWorkflow) error {
	if utils.HasFeature(t.Image, "WINDOWS") {
		t.Skip("OSLogin not supported on windows")
		return nil
	}

	// TODO(b/440641320): Remove this skip once the bug is fixed.
	if strings.Contains(t.Image.Name, "rhel-9-0-sap") {
		t.Skip("OSLogin is not working on rhel-9-0-sap images, skipping the test until b/440641320 is fixed.")
		return nil
	}

	// TODO(b/468323433): Remove this skip once the bug is fixed.
	if strings.Contains(t.Image.Name, "sles-16") || strings.Contains(t.Image.Name, "opensuse-leap-16") {
		t.Skip(fmt.Sprintf("OSLogin is not working on sles-16 images, skipping the test on %q until b/468323433 is fixed.", t.Image.Name))
		return nil
	}

	initialize2FA.Do(func() {
		ctx := context.Background()
		secretClient, err := secretmanager.NewClient(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create secret manager client: %v", err)
			return
		}
		normal2FARegex := regexp.MustCompile(`normal-2fa-user-(\d+)`)
		admin2FARegex := regexp.MustCompile(`admin-2fa-user-(\d+)`)
		secrets := secretClient.ListSecrets(ctx, &secretmanagerpb.ListSecretsRequest{Parent: fmt.Sprintf("projects/%s", t.Project.Name)})
		for secret, err := secrets.Next(); secret != nil && err != iterator.Done; secret, err = secrets.Next() {
			if normal2FARegex.MatchString(secret.Name) {
				num := normal2FARegex.FindStringSubmatch(secret.Name)[1]
				twoFATestUsers = append(twoFATestUsers, test2FAUser{
					email:  fmt.Sprintf("normal-2fa-user-%s", num),
					sshKey: fmt.Sprintf("normal-2fa-ssh-key-%s", num),
					twoFA:  fmt.Sprintf("normal-2fa-key-%s", num),
				})
			}
			if admin2FARegex.MatchString(secret.Name) {
				num := admin2FARegex.FindStringSubmatch(secret.Name)[1]
				twoFAAdminTestUsers = append(twoFAAdminTestUsers, test2FAUser{
					email:  fmt.Sprintf("admin-2fa-user-%s", num),
					sshKey: fmt.Sprintf("admin-2fa-ssh-key-%s", num),
					twoFA:  fmt.Sprintf("admin-2fa-key-%s", num),
				})
			}
		}

		// Randomize the starting point of the counter. We use the larger of the two
		// lists to avoid cases where one of the lists is extremely small, in which
		// case the randomization would not be very effective.
		counter = rand.Intn(int(math.Max(float64(len(twoFATestUsers)), float64(len(twoFAAdminTestUsers)))))
	})

	testAgent, err := t.CreateTestVM("testagent")
	if err != nil {
		return err
	}
	testAgent.AddScope(computeScope)
	testAgent.AddMetadata("enable-oslogin", "true")
	testAgent.AddMetadata("enable-oslogin-2fa", "true")
	testAgent.RunTests("TestAgent|TestOsLoginEnabled|TestGetentPasswd")

	defaultVM, err := t.CreateTestVM("default")
	if err != nil {
		return err
	}
	defaultVM.AddScope(computeScope)
	defaultVM.AddMetadata("enable-oslogin", "true")
	defaultVM.RunTests("TestEmpty")

	normalUser := twoFATestUsers[counter%len(twoFATestUsers)]
	adminUser := twoFAAdminTestUsers[counter%len(twoFAAdminTestUsers)]

	fmt.Printf("Normal User: %v\n", normalUser)
	fmt.Printf("Admin User: %v\n", adminUser)

	ssh, err := t.CreateTestVM("ssh")
	if err != nil {
		return err
	}
	ssh.AddScope(platformScope)
	ssh.AddMetadata("enable-oslogin", "false")
	ssh.AddMetadata(normal2FAUser, normalUser.email)
	ssh.AddMetadata(normal2FAKey, normalUser.twoFA)
	ssh.AddMetadata(normal2FASSHKey, normalUser.sshKey)
	ssh.AddMetadata(admin2FAUser, adminUser.email)
	ssh.AddMetadata(admin2FAKey, adminUser.twoFA)
	ssh.AddMetadata(admin2FASSHKey, adminUser.sshKey)
	ssh.RunTests("TestOsLoginDisabled|TestSSH|TestAdminSSH|Test2FASSH|Test2FAAdminSSH")

	twofa, err := t.CreateTestVM("twofa")
	if err != nil {
		return err
	}
	twofa.AddScope(computeScope)
	twofa.AddMetadata("enable-oslogin", "true")
	twofa.AddMetadata("enable-oslogin-2fa", "true")
	twofa.RunTests("TestEmpty")

	// This is used to stagger the admin users to avoid hitting 2FA quotas.
	counter++
	return nil
}
