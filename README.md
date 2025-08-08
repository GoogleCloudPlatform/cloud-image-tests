# Cloud Image Tests #

The **Cloud Image Tests** are a testing framework and a set of test suites used
for testing GCE Images.

## Invocation ##

Testing components are built into a container image. The entrypoint is
`/manager`, which supports the following options:

```
    Usage:
    -machine_type string
    	sets default machine_type for test runs, regardless of architecture
    	prefer the use -x86_shape and/or -arm64_shape instead
    -arm64_shape string
    	default arm64 vm shape for tests not requiring a specific shape (default "t2a-standard-1")
    -x86_shape string
    	default x86(-32 and -64) vm shape for tests not requiring a specific shape (default "n1-standard-1")
    -zone string
    	zone to be used for tests
    -project string
    	project to use for test runner
    -test_projects string
    	comma separated list of projects to be used for tests. Defaults to the test runner project
    -compute_endpoint_override string
    	use a different endpoint for compute client libraries
     -exclude string
    	skip tests matching filter
    -filter string
    	only run tests matching filter
    -exclude_discrete_tests string
        skip individual tests within the suite that match the filter
    -gcs_path string
    	GCS Path for Daisy working directory
    -images string
    	comma separated list of images to test. These can be fully qualified
        image URLs (like "projects/my-project/global/images/my-image" or
        "projects/my-project/global/images/family/my-family") or just the name
        of the family if the family is a standard image (like "debian-12")
    -local_path string
    	path where test binaries are stored
    -out_path string
    	junit xml output path (default "junit.xml")
    -write_local_artifacts string
    	Local path to download test artifacts from gcs. (default none)
    -parallel_count int
    	tests to run at one time
    -parallel_stagger string
    	parsable time.Duration to stagger each parallel test (default "60s")
    -set_exit_status
    	Exit with non-zero exit code if test suites are failing (default true)
    -timeout string
    	timeout for each step in the test workflow (default "20m")
    -print
    	instead of running, print out the parsed test workflows and exit
    -validate
    	validate all the test workflows and exit
```

The following flags are provided to the manager but interpreted by test suites
when run, see the [the `test_suites` documentation](test_suites/README.md) for
more information.

```
    -shapevalidation_test_filter string
    	regexp filter for shapevalidation test cases, only cases with a matching family name will be run (default ".*")
    -storageperf_test_filter string
    	regexp filter for storageperf test cases, only cases with a matching name will be run (default ".*")
    -networkinterfacenaming_metal_zone string
        zone in which to create the C3 Metal instance for images supporting IDPF. For zones with availability, refer to https://cloud.google.com/compute/docs/general-purpose-machines#c3_regions.
    -networkperf_test_filter string
    	regexp filter for networkperf test cases, only cases with a matching name will be run (default ".*")
    -nicsetup_vmtype string
        string indicating type of VMs to create for nicsetup test cases.
        Valid values are "both", "single", and "multi". "single" creates only
        single-NIC VMs, "multi" creates only multi-NIC VMs, and "both" creates
        both (default "both")
```

It can be invoked via Docker as:

```shell
images="projects/debian-cloud/global/images/family/debian-11,rhel-9"
docker run gcr.io/cloud-image-tools/cloud-image-tests --project $PROJECT \
    --zone $ZONE --images $images
```

### Credentials ###

The test manager is designed to be run in a Google Cloud environment, and will
use application default credentials. If you are not in a Google Cloud
environment and need to specify the credentials to use, you can provide them as
a docker volume and specify the path with the `GOOGLE_APPLICATION_CREDENTIALS`
environment variable.

Assuming your application default or service account credentials are in a file
named credentials.json:

```shell
docker run -v /path/to/local/creds:/creds \
    -e GOOGLE_APPLICATION_CREDENTIALS=/creds/credentials.json \
    gcr.io/gcp-guest/cloud-image-tests -project $PROJECT \
    -zone $ZONE -images $images
```

The manager will exit with 0 if all tests completed successfully, 1 otherwise.
JUnit format XML will also be output.

## Writing tests ##

Tests are organized into go packages in the `test_suites` directory and are
written in go. Each package must at a minimum contain a setup file (by
convention named setup.go) and at least one test file (by convention named
`$packagename_test.go`). Due to Golang style conventions, the package name
cannot contain an underscore. Thus, for the test suite name to match the package
name, the name of the test suite should not contain an underscore. For example,
if a new test suite was created to test image licenses, it should be called
`imagelicensing`, not `image_licensing`.

The `setup.go` file describes the workflow to run including the VMs and other
GCE resources to create, any necessary configuration for those resources, which
specific tests to run, etc.. It is here where you can also skip an entire test
package based on inputs e.g. image, zone or compute endpoint or other
conditions.

Tests themselves are written in the test file(s) as go unit tests. Tests may use
any of the test fixtures provided by the standard `testing` package. These will
be packaged into a binary and run on the test VMs created during setup using the
Google Compute Engine startup script runner.

When writing tests to run against both Linux and Windows, it is preferred to
use separate functions within the same test where appropriate based on
differences between OSes (ex. powershell vs bash commands). This makes the
test definitions easier to read and maintain.

For example, if the test `TestSomeCondition()` needs to run different commands
to achieve similar results (and the test is located in the directory
`mydefaulttest`):

```go

package mydefaulttest

import (
    "runtime"
    "testing"
)

func RunTestConditionWindows() {
    //Test something in Windows
}
func RunTestCondition() {
    //Test something in Linux
}

func TestSomeCondition(t *testing.T) {
    if runtime.GOOS == "windows" {
    RunTestConditionWindows()
    } else {
        RunTestCondition()
    }
}
```

If there are functions or libraries with differences across OSes, then you
should split the relevant functions across different OSes (an example of such a
library is the internal `syscall` library). For example, if in the Windows
library `X` defines constant `Y` (but not `Z`), but in the Linux library it
defines constant `Z` (but not `Y`), then the test can look something like the
following

`mydefaulttest.go`:
```go
package mydefaulttest

import (
    "testing"
)

func TestSomeCondition(t *testing.T) {
    runTestCondition(t)
}
```

`mydefaulttest_windows.go`
```go
//go:build windows

package mydefaulttest

import (
    "testing"
    "X"
)

func runTestCondition(t *testing.T) {
    t.Helper()

    testVar := X.Y
    // Test something on Windows.
}
```

`mydefaulttest_linux.go`
```go
//go:build linux

package mydefaulttest

import (
    "testing"
    "X"
)

func runTestCondition(t *testing.T) {
    t.Helper()

    testVar := X.Z
    // Test something on Linux.
}
```

Note that there are also functions available in the [test_utils.go](utils/test_utils.go)
for some OS level abstractions such as running a Windows powershell command or
checking if a Linux binary exists.

It is suggested to start by copying an existing test package. Do not forget to
add your test to the relevant `setup.go` file in order to add the test to the
test suite.

### Modifying test behavior based on image properties ###

For tests that need to behave different based on whether an image is arm or x86,
or linux or windows, it is preferred to use compute API properties rather than
relying on image naming conventions. These properties can be found on the
testworkflow Image value. The list of values can be found in the Compute API
documentation [here](https://pkg.go.dev/google.golang.org/api/compute/v1#Image).
Some examples are in the following code snippet.

```go
func Setup(t *imagetest.Testworkflow) {
	if t.Image.Architecture == "ARM64" {
	//...
	} else if utils.HasFeature(t.Image, "GVNIC") {
	//...
	}
}
```

For tests that need to either skip a test case or modify its behavior based on
the image it's running, you can use the `utils/exceptions` library to define
them. You can refer to the implementation [here](https://github.com/GoogleCloudPlatform/cloud-image-tests/blob/main/utils/exceptions/exceptions.go)

### Testing features in compute beta API ###

Tests that need to run against features in the beta API can do so by creating
TestVMs using `CreateTestVMBeta` or `CreateTestVMFromInstanceBeta` to use the
beta instance API. However, due to limitation with daisy's create instances
step, if one instance in a TestWorkflow uses the beta API all instances in that
workflow must use the beta API.

## Building and running the container image ##

From the root directory of this repository:

```shell
docker build -t cloud-image-tests -f Dockerfile .
```

To run the locally-built Docker image:

```shell
docker run cloud-image-tests --project $PROJECT \
    --zone $ZONE --images $images
```

<!-- disableFinding(LINK_ID) -->
Make sure to define and include the
[application default credentials](#credentials) if needed.
<!-- enableFinding(LINK_ID) -->

## Testing on a local machine ##

From the `imagetest` directory of this repository, where outspath is
the folder where test outputs are stored:

```shell
local_build.sh -o $outspath
```

By default, all test suites are built. To build only one test suite:

```shell
local_build.sh -o $outspath -s $test_suite_name
```

To build from a directory other than `imagetest`

```shell
local_build.sh -o $outspath -i $path_to_imagetest
```

To run the tests, cd into $outspath, set the shell variables and run

```shell
manager -zone $ZONE -project $PROJECT -images $images -filter $test_suite_name -local_path .
```

## What is being tested ##

The tests are a combination of various types - end to end tests on certain
software components, image validations and feature validations, etc. The
collective whole represents the quality assurance bar for releasing a [supported
GCE Image][gce-images], and the test suites here must all pass before Google
engineers will release a new GCE image.

The tests are documented in [the `test_suites` directory](test_suites/README.md).
