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

// Wrapper is the binary executed inside the test VM. It fetches the test
// binary, executes it, and uploads its results to GCS.
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
	vm_pb "github.com/GoogleCloudPlatform/cloud-image-tests/vm_test_info"
	"google.golang.org/protobuf/proto"
)

// In special cases such as the shutdown script, the guest attribute match
// on the first boot must have a different name than the usual guest attribute.
func checkFirstBootSpecialGA(ctx context.Context) bool {
	if _, err := utils.GetMetadata(ctx, "instance", "attributes", "shouldRebootDuringTest"); err == nil {
		_, foundFirstBootGA := utils.GetMetadata(ctx, "instance", "guest-attributes",
			utils.GuestAttributeTestNamespace, utils.FirstBootGAKey)
		// if the special attribute to match the first boot of the shutdown script test is already set, foundFirstBootGA will be nil and we should use the regular guest attribute.
		if foundFirstBootGA != nil {
			return true
		}
	}
	return false
}

func main() {
	ctx := context.Background()
	var cancel context.CancelFunc

	var testBinaryTimeout string
	vmDeadline, err := utils.FindVMDeadline(ctx)
	if err != nil {
		testBinaryTimeout = "0"
		log.Printf("could not find timeout: %v", err)
	} else {
		ctx, cancel = context.WithDeadline(ctx, vmDeadline)
		defer cancel()
	}

	client, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("failed to create cloud storage client: %v", err)
	}

	log.Printf("FINISHED-BOOTING")
	firstBootSpecialAttribute := checkFirstBootSpecialGA(ctx)
	// firstBootSpecialGA should be true if we need to match a different guest attribute than the usual guest attribute
	defer func(ctx context.Context, firstBootSpecialGA bool) {
		var err error
		if firstBootSpecialGA {
			err = utils.PutMetadata(ctx, path.Join("instance", "guest-attributes", utils.GuestAttributeTestNamespace,
				utils.FirstBootGAKey), "")
		} else {
			err = utils.PutMetadata(ctx, path.Join("instance", "guest-attributes", utils.GuestAttributeTestNamespace,
				utils.GuestAttributeTestKey), "")
		}

		if err != nil {
			log.Printf("could not place guest attribute key to end test: %v", err)
		}

		for f := 0; f < 5; f++ {
			log.Printf("FINISHED-TEST")
			time.Sleep(1 * time.Second)
		}
	}(ctx, firstBootSpecialAttribute)

	testPackageURL, err := utils.GetMetadata(ctx, "instance", "attributes", "_test_package_url")
	if err != nil {
		log.Fatalf("failed to get metadata _test_package_url: %v", err)
	}

	resultsURL, err := utils.GetMetadata(ctx, "instance", "attributes", "_test_results_url")
	if err != nil {
		log.Fatalf("failed to get metadata _test_results_url: %v", err)
	}

	propertiesURL, err := utils.GetMetadata(ctx, "instance", "attributes", "_test_properties_url")
	if err != nil {
		log.Fatalf("failed to get metadata _test_properties_url: %v", err)
	}

	var testArguments = []string{"-test.v"}

	testRun, err := utils.GetMetadata(ctx, "instance", "attributes", "_test_run")
	if err == nil && testRun != "" {
		testArguments = append(testArguments, "-test.run", testRun)
	}

	testExcludeFilter, err := utils.GetMetadata(ctx, "instance", "attributes", "_exclude_discrete_tests")
	if err == nil && testExcludeFilter != "" {
		testArguments = append(testArguments, "-test.skip", testExcludeFilter)
	}

	testPackage, err := utils.GetMetadata(ctx, "instance", "attributes", "_test_package_name")
	if err != nil {
		log.Fatalf("failed to get metadata _test_package_name: %v", err)
	}

	testSuiteName, err := utils.GetMetadata(ctx, "instance", "attributes", "_test_suite_name")
	if err != nil {
		log.Fatalf("failed to get metadata _test_suite_name: %v", err)
	}

	machineType, err := utils.GetMetadata(ctx, "instance", "machine-type")
	if err != nil {
		log.Fatalf("failed to get metadata _machine_type: %v", err)
	}

	zone, err := utils.GetMetadata(ctx, "instance", "zone")
	if err != nil {
		log.Fatalf("failed to get metadata _zone: %v", err)
	}

	id, err := utils.GetMetadata(ctx, "instance", "id")
	if err != nil {
		log.Fatalf("failed to get metadata _id: %v", err)
	}

	name, err := utils.GetMetadata(ctx, "instance", "name")
	if err != nil {
		log.Fatalf("failed to get metadata _name: %v", err)
	}

	workDirPath := "/etc/"
	if runtime.GOOS == "windows" {
		workDirPath = "C:\\"
	}
	workDir, err := os.MkdirTemp(workDirPath, "image_test")
	if err != nil {
		log.Fatalf("failed to create work dir: %v", err)
	}
	workDir = workDir + "/"

	if err = utils.DownloadGCSObjectToFile(ctx, client, testPackageURL, workDir+testPackage); err != nil {
		log.Fatalf("failed to download object: %v", err)
	}
	client.Close()

	log.Printf("sleep 30s to allow environment to stabilize")
	time.Sleep(30 * time.Second)

	if testBinaryTimeout != "0" {
		// One would think that this should be computed from TestBinaryDeadline
		// instead of VMDeadline, but no. This is a hard deadline by which the
		// binary must exit, the test will panic if it hits this and we will have
		// no results. The test binary internally uses a shorter deadline to report
		// results.
		testBinaryTimeout = fmt.Sprintf("%ds", int64(time.Until(vmDeadline).Seconds()))
	}
	testArguments = append(testArguments, "-test.timeout", testBinaryTimeout)
	out, err := executeCmd(workDir+testPackage, workDir, testArguments)
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			log.Printf("test package exited with error: %v stderr: %q", ee, ee.Stderr)
		} else {
			log.Fatalf("failed to execute test package: %v stdout: %q", err, out)
		}
	}

	log.Printf("command output:\n%s\n", out)

	client, err = storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("failed to create cloud storage client: %v", err)
	}
	defer client.Close()
	if err = uploadGCSObject(ctx, client, resultsURL, bytes.NewReader(out)); err != nil {
		log.Fatalf("failed to upload test result: %v", err)
	}

	vmInfoProto := &vm_pb.Vm{
		Test: &vm_pb.Vm_Test{
			TestSuite: proto.String(testSuiteName),
			TestRegex: proto.String(testRun),
		},
		Name:        proto.String(name),
		Id:          proto.String(id),
		Zone:        proto.String(zone),
		MachineType: proto.String(machineType),
	}

	vmInfo, err := proto.Marshal(vmInfoProto)
	if err != nil {
		log.Fatalf("failed to marshal vm info proto: %v", err)
	}
	if err = uploadGCSObject(ctx, client, propertiesURL, bytes.NewReader(vmInfo)); err != nil {
		log.Fatalf("failed to upload vm info: %v", err)
	}
}

func executeCmd(cmd, dir string, arg []string) ([]byte, error) {
	command := exec.Command(cmd, arg...)
	command.Dir = dir
	log.Printf("Going to execute: %q", command.String())

	output, err := command.Output()
	if err != nil {
		return output, err
	}
	return output, nil
}

func uploadGCSObject(ctx context.Context, client *storage.Client, path string, data io.Reader) error {
	u, err := url.Parse(path)
	if err != nil {
		log.Fatalf("failed to parse gcs url: %v", err)
	}
	object := strings.TrimPrefix(u.Path, "/")
	log.Printf("uploading to bucket %s object %s\n", u.Host, object)

	upload := func() error {
		dst := client.Bucket(u.Host).Object(object).NewWriter(ctx)
		if _, err := io.Copy(dst, data); err != nil {
			return fmt.Errorf("failed to write to gcs: %w", err)
		}
		if err := dst.Close(); err != nil {
			return fmt.Errorf("failed to close gcs writer: %w", err)
		}
		return nil
	}

	var uploadErr error
	for i := 1; i <= 5; i++ {
		if uploadErr = upload(); uploadErr != nil {
			time.Sleep(time.Duration(i) * time.Second)
			continue
		}
		break
	}

	return uploadErr
}
