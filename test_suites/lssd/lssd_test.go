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

package lssd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

func getWindowsDiskNumber(ctx context.Context) (int, error) {
	diskName, err := utils.GetMetadata(ctx, "instance", "attributes", "hotattach-disk-name")
	if err != nil {
		return 0, err
	}
	intMatch := regexp.MustCompile("[0-9]+")
	o, err := utils.RunPowershellCmd(`(Get-Disk -FriendlyName "` + diskName + `").Number`)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(intMatch.FindString(o.Stdout))
}

func getLinuxMountPath(ctx context.Context) (string, error) {
	diskName, err := utils.GetMetadata(ctx, "instance", "attributes", "hotattach-disk-name")
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks("/dev/disk/by-id/google-" + diskName)
}

func mountLinuxDiskToPath(ctx context.Context, mountDiskDir string, isReattach bool) error {
	if err := os.MkdirAll(mountDiskDir, 0777); err != nil {
		return fmt.Errorf("could not make mount disk dir %s: error %v", mountDiskDir, err)
	}
	mountDiskPath, err := getLinuxMountPath(ctx)
	if err != nil {
		return err
	}
	if !utils.CheckLinuxCmdExists(mkfsCmd) {
		return fmt.Errorf("could not format mount disk: %s cmd not found", mkfsCmd)
	}
	if !isReattach {
		mkfsFullCmd := exec.Command(mkfsCmd, "-m", "0", "-E", "lazy_itable_init=0,lazy_journal_init=0,discard", "-F", mountDiskPath)
		if stdout, err := mkfsFullCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("mkfs cmd failed to complete: %v %v", stdout, err)
		}
	}

	mountCmd := exec.Command("mount", "-o", "discard,defaults", mountDiskPath, mountDiskDir)

	if err := mountCmd.Run(); err != nil {
		return fmt.Errorf("failed to mount disk: %v", err)
	}

	return nil
}

// TestMount tests that a drive can be mounted and written to. Hotattach without the attaching and detaching.
func TestMount(t *testing.T) {
	ctx, cancel := utils.Context(t)
	defer cancel()
	fileName := "hotattach.txt"
	fileContents := "cold Attach"
	fileContentsBytes := []byte(fileContents)
	var fileFullPath string
	if runtime.GOOS == "windows" {
		diskNum, err := getWindowsDiskNumber(ctx)
		if err != nil {
			diskNum = 1
		}
		procStatus, err := utils.RunPowershellCmd(fmt.Sprintf(`Initialize-Disk -PartitionStyle GPT -Number %d -PassThru | New-Partition -DriveLetter %s -UseMaximumSize | Format-Volume -FileSystem NTFS -NewFileSystemLabel 'Attach-Test' -Confirm:$false`, diskNum, windowsMountDriveLetter))
		if err != nil {
			t.Fatalf("failed to initialize disk on windows: errors %v, %s, %s", err, procStatus.Stdout, procStatus.Stderr)
		}
		fileFullPath = windowsMountDriveLetter + ":\\" + fileName
	} else {
		if err := mountLinuxDiskToPath(ctx, linuxMountPath, false); err != nil {
			t.Fatalf("failed to mount linux disk to linuxmountpath %s: error %v", linuxMountPath, err)
		}
		fileFullPath = linuxMountPath + "/" + fileName
	}
	f, err := os.Create(fileFullPath)
	if err != nil {
		f.Close()
		t.Fatalf("failed to create file at path %s: error %v", fileFullPath, err)
	}

	w := bufio.NewWriter(f)
	_, err = w.Write(fileContentsBytes)
	if err != nil {
		f.Close()
		t.Fatalf("failed to write bytes: err %v", err)
	}
	w.Flush()
	f.Sync()

	hotAttachFile, err := os.Open(fileFullPath)
	if err != nil {
		hotAttachFile.Close()
		t.Fatalf("file could not be reopened at path %s: error A%v", fileFullPath, err)
	}
	defer hotAttachFile.Close()

	fileLength, err := hotAttachFile.Read(fileContentsBytes)
	if fileLength == 0 {
		t.Fatalf("file was empty after writing to it")
	}
	if err != nil {
		t.Fatalf("reading file after writing failed with error: %v", err)
	}
}
