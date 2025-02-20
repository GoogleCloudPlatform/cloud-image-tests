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

// Manager is a cli interface to the orchestration provided by the imagetest
// library. Run this binary with the desired flags to run a test workflow on an
// image.
package main

import (
	"context"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/GoogleCloudPlatform/cloud-image-tests"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/acceleratorconfig"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/acceleratorrdma"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/compatmanager"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/cvm"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/disk"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/guestagent"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/hostnamevalidation"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/hotattach"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/imageboot"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/licensevalidation"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/livemigrate"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/loadbalancer"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/lssd"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/mdsmtls"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/mdsroutes"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/metadata"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/network"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/networkinterfacenaming"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/networkperf"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/oslogin"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/packagemanager"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/packageupgrade"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/packagevalidation"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/pluginmanager"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/security"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/shapevalidation"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/sql"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/ssh"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/storageperf"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/suspendresume"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/vmspec"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/windowscontainers"
	"github.com/GoogleCloudPlatform/cloud-image-tests/test_suites/winrm"
	"github.com/GoogleCloudPlatform/compute-daisy/compute"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

var (
	project                 = flag.String("project", "", "project to use for test runner")
	testProjects            = flag.String("test_projects", "", "comma separated list of projects to be used for tests. defaults to the test runner project")
	zone                    = flag.String("zone", "us-central1-a", "zone to be used for tests")
	printwf                 = flag.Bool("print", false, "print out the parsed test workflows and exit")
	validate                = flag.Bool("validate", false, "validate all the test workflows and exit")
	outPath                 = flag.String("out_path", "junit.xml", "junit xml path")
	gcsPath                 = flag.String("gcs_path", "", "GCS Path for Daisy working directory")
	writeLocalArtifacts     = flag.String("write_local_artifacts", "", "Local path to download test artifacts from gcs.")
	localPath               = flag.String("local_path", "", "path where test output files are stored, can be modified for local testing")
	images                  = flag.String("images", "", "comma separated list of images to test")
	timeout                 = flag.String("timeout", "45m", "timeout for the test suite")
	computeEndpointOverride = flag.String("compute_endpoint_override", "", "compute client endpoint override")
	parallelCount           = flag.Int("parallel_count", 5, "TestParallelCount")
	parallelStagger         = flag.String("parallel_stagger", "60s", "parseable time.Duration to stagger each parallel test")
	filter                  = flag.String("filter", "", "only run test suites matching filter")
	exclude                 = flag.String("exclude", "", "skip test suites matching filter")
	testExcludeFilter       = flag.String("exclude_discrete_tests", "", "skip individual tests within suites that match the regexp filter")
	machineType             = flag.String("machine_type", "", "deprecated, use -x86_shape and/or -arm64_shape instead")
	x86Shape                = flag.String("x86_shape", "n1-standard-1", "default x86(-32 and -64) vm shape for tests not requiring a specific shape")
	arm64Shape              = flag.String("arm64_shape", "t2a-standard-1", "default arm64 vm shape for tests not requiring a specific shape")
	setExitStatus           = flag.Bool("set_exit_status", true, "Exit with non-zero exit code if test suites are failing")
	useReservations         = flag.Bool("use_reservations", false, "Whether to consume reservations when creating VMs. Will consume any reservation if reservation_urls is unspecified.")
	reservationURLs         = flag.String("reservation_urls", "", "Comma separated list of partial URLs for reservations to consume.")
)

var (
	projectMap = map[string]string{
		"almalinux":     "almalinux-cloud",
		"centos":        "centos-cloud",
		"cos":           "cos-cloud",
		"debian":        "debian-cloud",
		"fedora-cloud":  "fedora-cloud",
		"fedora-coreos": "fedora-coreos-cloud",
		"opensuse":      "opensuse-cloud",
		"rhel":          "rhel-cloud",
		"rhel-sap":      "rhel-sap-cloud",
		"rocky-linux":   "rocky-linux-cloud",
		"sles":          "suse-cloud",
		"sles-sap":      "suse-sap-cloud",
		"sql-":          "windows-sql-cloud",
		"ubuntu":        "ubuntu-os-cloud",
		"ubuntu-pro":    "ubuntu-os-pro-cloud",
		"windows":       "windows-cloud",
	}
)

type logWriter struct {
	log *log.Logger
}

func (l *logWriter) Write(b []byte) (int, error) {
	l.log.Print(string(b))
	return len(b), nil
}

func main() {
	flag.Parse()
	if *project == "" || *zone == "" || *images == "" {
		log.Fatal("Must provide project, zone and images arguments")
		return
	}
	var testProjectsReal []string
	if *testProjects == "" {
		testProjectsReal = append(testProjectsReal, *project)
	} else {
		testProjectsReal = strings.Split(*testProjects, ",")
	}

	log.Printf("Running in project %s zone %s. Tests will run in projects: %s", *project, *zone, testProjectsReal)
	if *gcsPath != "" {
		log.Printf("gcs_path set to %s", *gcsPath)
	}

	var filterRegex *regexp.Regexp
	if *filter != "" {
		var err error
		filterRegex, err = regexp.Compile(*filter)
		if err != nil {
			log.Fatal("-filter flag not valid:", err)
		}
		log.Printf("using -filter %s", *filter)
	}

	var excludeRegex *regexp.Regexp
	if *exclude != "" {
		var err error
		excludeRegex, err = regexp.Compile(*exclude)
		if err != nil {
			log.Fatal("-exclude flag not valid:", err)
		}
		log.Printf("using -exclude %s", *exclude)
	}

	if *testExcludeFilter != "" {
		log.Printf("Using -exclude_discrete_tests %s", *testExcludeFilter)
	}

	if *machineType != "" {
		log.Printf("The -machine_type flag is deprecated, please use -x86_shape and -arm64_shape instead. Retaining legacy behavior while this is set.")
		*x86Shape = *machineType
		*arm64Shape = *machineType
	}

	var reservationURLSlice []string
	if *reservationURLs != "" {
		reservationURLSlice = strings.Split(*reservationURLs, ",")
	}

	// Setup tests.
	testPackages := []struct {
		name      string
		setupFunc func(*imagetest.TestWorkflow) error
	}{
		{
			acceleratorconfig.Name,
			acceleratorconfig.TestSetup,
		},
		{
			acceleratorrdma.Name,
			acceleratorrdma.TestSetup,
		},
		{
			cvm.Name,
			cvm.TestSetup,
		},
		{
			livemigrate.Name,
			livemigrate.TestSetup,
		},
		{
			suspendresume.Name,
			suspendresume.TestSetup,
		},
		{
			networkperf.Name,
			networkperf.TestSetup,
		},

		{
			networkinterfacenaming.Name,
			networkinterfacenaming.TestSetup,
		},
		{
			loadbalancer.Name,
			loadbalancer.TestSetup,
		},
		{
			guestagent.Name,
			guestagent.TestSetup,
		},
		{
			hostnamevalidation.Name,
			hostnamevalidation.TestSetup,
		},
		{
			imageboot.Name,
			imageboot.TestSetup,
		},
		{
			licensevalidation.Name,
			licensevalidation.TestSetup,
		},
		{
			network.Name,
			network.TestSetup,
		},
		{
			security.Name,
			security.TestSetup,
		},
		{
			hotattach.Name,
			hotattach.TestSetup,
		},
		{
			lssd.Name,
			lssd.TestSetup,
		},
		{
			disk.Name,
			disk.TestSetup,
		},
		{
			shapevalidation.Name,
			shapevalidation.TestSetup,
		},
		{
			packagemanager.Name,
			packagemanager.TestSetup,
		},
		{
			packageupgrade.Name,
			packageupgrade.TestSetup,
		},
		{
			packagevalidation.Name,
			packagevalidation.TestSetup,
		},
		{
			storageperf.Name,
			storageperf.TestSetup,
		},
		{
			ssh.Name,
			ssh.TestSetup,
		},
		{
			winrm.Name,
			winrm.TestSetup,
		},
		{
			sql.Name,
			sql.TestSetup,
		},
		{
			metadata.Name,
			metadata.TestSetup,
		},
		{
			oslogin.Name,
			oslogin.TestSetup,
		},
		{
			mdsmtls.Name,
			mdsmtls.TestSetup,
		},
		{
			mdsroutes.Name,
			mdsroutes.TestSetup,
		},
		{
			windowscontainers.Name,
			windowscontainers.TestSetup,
		},
		{
			vmspec.Name,
			vmspec.TestSetup,
		},
		{
			pluginmanager.Name,
			pluginmanager.TestSetup,
		},
		{
			compatmanager.Name,
			compatmanager.TestSetup,
		},
	}

	ctx := context.Background()
	var computeclient compute.Client
	var err error
	if *computeEndpointOverride != "" {
		log.Printf("Using compute endpoint %q", *computeEndpointOverride)
		computeclient, err = compute.NewClient(ctx, option.WithEndpoint(*computeEndpointOverride))
	} else {
		computeclient, err = compute.NewClient(ctx)
	}
	if err != nil {
		log.Fatalf("Could not create compute client:%v", err)
	}

	var testWorkflows []*imagetest.TestWorkflow
	for _, testPackage := range testPackages {
		if filterRegex != nil && !filterRegex.MatchString(testPackage.name) {
			continue
		}
		if excludeRegex != nil && excludeRegex.MatchString(testPackage.name) {
			continue
		}
		for _, image := range strings.Split(*images, ",") {
			if !strings.Contains(image, "/") {
				// Find the project of the image.
				project := ""
				for k := range projectMap {
					if strings.Contains(k, "sap") {
						// sap follows a slightly different naming convention.
						imageName := strings.Split(k, "-")[0]
						if strings.HasPrefix(image, imageName) && strings.Contains(image, "sap") {
							project = projectMap[k]
							break
						}
					}
					if strings.HasPrefix(image, k) {
						project = projectMap[k]
						break
					}
				}
				if project == "" {
					log.Fatalf("unknown image %s", image)
				}

				// Check whether the image is an image family or a specific image version.
				isMatch, err := regexp.MatchString(".*v([0-9]+)", image)
				if err != nil {
					log.Fatalf("failed regex: %v", err)
				}
				if isMatch {
					image = fmt.Sprintf("projects/%s/global/images/%s", project, image)
				} else {
					image = fmt.Sprintf("projects/%s/global/images/family/%s", project, image)
				}
			}

			log.Printf("Add test workflow for test %s on image %s", testPackage.name, image)
			test, err := imagetest.NewTestWorkflow(&imagetest.TestWorkflowOpts{
				Client:                  computeclient,
				ComputeEndpointOverride: *computeEndpointOverride,
				Name:                    testPackage.name,
				Image:                   image,
				Timeout:                 *timeout,
				Project:                 *project,
				Zone:                    *zone,
				ExcludeFilter:           *testExcludeFilter,
				X86Shape:                *x86Shape,
				ARM64Shape:              *arm64Shape,
				UseReservations:         *useReservations,
				ReservationURLs:         reservationURLSlice,
			})
			if err != nil {
				log.Fatalf("Failed to create test workflow: %v", err)
			}
			testWorkflows = append(testWorkflows, test)
			if err := testPackage.setupFunc(test); err != nil {
				log.Fatalf("%s.TestSetup for %s failed: %v", testPackage.name, image, err)
			}
		}
	}

	if len(testWorkflows) == 0 {
		log.Fatalf("No workflows to run!")
	}

	log.Println("Done with setup")

	storageclient, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("failed to set up storage client: %v", err)
	}

	if *printwf {
		imagetest.PrintTests(ctx, storageclient, testWorkflows, *project, *zone, *gcsPath, *localPath)
		return
	}

	if *validate {
		if err := imagetest.ValidateTests(ctx, storageclient, testWorkflows, *project, *zone, *gcsPath, *localPath); err != nil {
			log.Printf("Validate failed: %v\n", err)
		}
		return
	}

	suites, err := imagetest.RunTests(ctx, storageclient, testWorkflows, *project, *zone, *gcsPath, *localPath, *parallelCount, *parallelStagger, testProjectsReal)
	if err != nil {
		log.Fatalf("Failed to run tests: %v", err)
	}
	if *writeLocalArtifacts != "" {
		var wg sync.WaitGroup
		for _, twf := range testWorkflows {
			bkt := strings.TrimSuffix(strings.TrimPrefix(regexp.MustCompile(`gs://[a-z0-9][a-z0-9-_.]{2,62}[a-z0-9]/?`).FindString(twf.GCSPath), "gs://"), "/")
			if bkt == "" {
				log.Printf("could not find gcs bucket from %s for workflow %s", twf.GCSPath, twf.Name)
				continue
			}
			gcsSubfolder := strings.TrimPrefix(twf.GCSPath, "gs://"+bkt+"/")
			wg.Add(1)
			go func(bucket, folder, dstDir string) {
				defer wg.Done()
				if err := downloadFolder(ctx, storageclient, bucket, folder, dstDir); err != nil {
					log.Printf("failed to download test artifacts from folder %s in bucket %s to %s: %v\n", folder, bucket, dstDir, err)
				}
			}(bkt, gcsSubfolder, *writeLocalArtifacts)
		}
		wg.Wait()
	}

	bytes, err := xml.MarshalIndent(suites, "", "\t")
	if err != nil {
		log.Fatalf("failed to marshall result: %v", err)
	}
	bytes = []byte(fmt.Sprintf("%s%s", xml.Header, bytes))
	var outFile *os.File
	if artifacts := os.Getenv("ARTIFACTS"); artifacts != "" {
		outFile, err = os.Create(artifacts + "/junit.xml")
	} else {
		outFile, err = os.Create(*outPath)
	}
	if err != nil {
		log.Fatalf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	outFile.Write(bytes)
	outFile.Write([]byte{'\n'})
	fmt.Printf("%s\n", bytes)

	if *setExitStatus && (suites.Errors != 0 || suites.Failures != 0) {
		log.Fatalf("test suite has error or failure")
	}
}

func downloadFolder(ctx context.Context, client *storage.Client, bucket, folder, dstDir string) error {
	// Create the destination directory if it doesn't exist.
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return err
	}

	// List all objects in the folder.
	query := &storage.Query{
		Prefix: folder,
	}
	objs := client.Bucket(bucket).Objects(ctx, query)

	// Download each object to the destination directory.
	for {
		obj, err := objs.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		dstFile := filepath.Join(dstDir, obj.Name)
		if strings.Contains(dstFile, "/sources/") {
			continue
		}
		// Remote path might contain subfolders, create them locally too.
		fileDstDir := filepath.Dir(dstFile)
		if err := os.MkdirAll(fileDstDir, 0755); err != nil {
			log.Printf("failed to create %s: %v", fileDstDir, err)
			continue
		}
		file, err := os.Create(dstFile)
		if err != nil {
			log.Printf("failed to write %s: %v", dstFile, err)
			continue
		}

		objReader, err := client.Bucket(bucket).Object(obj.Name).NewReader(ctx)
		if err != nil {
			log.Printf("failed to make reader for %s: %v", obj.Name, err)
			continue
		}

		if _, err := io.Copy(file, objReader); err != nil {
			log.Printf("failed to copy %s to disk: %v", obj.Name, err)
			continue
		}

		if err := objReader.Close(); err != nil {
			log.Printf("failed to close %s reader: %v", obj.Name, err)
			continue
		}
	}

	return nil
}
