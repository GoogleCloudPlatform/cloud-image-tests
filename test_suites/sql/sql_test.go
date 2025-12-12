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

package sql

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/cloud-image-tests/utils"
)

type buildVersionBounds struct {
	upper int
	lower int
}

var serverVersionBuildNumber = map[string]buildVersionBounds{
	"2016": {upper: 17763, lower: 14392},
	"2019": {upper: 20348, lower: 17762},
	"2022": {upper: 26000, lower: 20347},
	// TODO change this to an actual upper bound when possible.
	"2025": {upper: 99999, lower: 25999},
}

func buildNumberIsOk(sqlOutput, serverExpectedVersion string) bool {
	bounds, ok := serverVersionBuildNumber[serverExpectedVersion]
	if !ok || (bounds.upper == 0 && bounds.lower == 0) {
		return false
	}
	buildNumString := regexp.MustCompile(`build [0-9]+:`).FindString(sqlOutput)
	if buildnumber, err := strconv.Atoi(regexp.MustCompile(`[0-9]+`).FindString(buildNumString)); err == nil {
		return buildnumber < bounds.upper && buildnumber > bounds.lower
	}
	return false
}

func TestSqlVersion(t *testing.T) {
	utils.WindowsOnly(t)

	image, err := utils.GetMetadata(utils.Context(t), "instance", "image")
	if err != nil {
		t.Fatal("Failed to get image metadata")
	}

	imageName, err := utils.ExtractBaseImageName(image)
	if err != nil {
		t.Fatal(err)
	}

	imageNameSplit := strings.Split(imageName, "-")
	sqlExpectedVer := imageNameSplit[1]
	sqlExpectedEdition := imageNameSplit[2]
	serverExpectedVer := imageNameSplit[4]

	command := fmt.Sprintf("Sqlcmd -C -Q \"select @@version\"")
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Unable to query SQL Server version: %v %v %v", output.Stdout, output.Stderr, err)
	}

	sqlOutput := strings.ToLower(strings.TrimSpace(output.Stdout))

	if !strings.Contains(sqlOutput, sqlExpectedEdition) {
		t.Fatalf("Installed SQL Server edition does not match image edition: %s not found in %s", sqlExpectedEdition, sqlOutput)
	}

	sqlVerString := "microsoft sql server " + sqlExpectedVer
	if !strings.Contains(sqlOutput, sqlVerString) {
		t.Fatalf("Installed SQL Server version does not match image version: %s not found in %s", sqlVerString, sqlOutput)
	}

	serverVerString := "on windows server " + serverExpectedVer
	if !strings.Contains(sqlOutput, serverVerString) && !buildNumberIsOk(sqlOutput, serverExpectedVer) {
		t.Fatalf("Installed Windows Server version does not match image version: year %s or build number between %d and %d not found in %s", serverVerString, serverVersionBuildNumber[serverExpectedVer].lower, serverVersionBuildNumber[serverExpectedVer].upper, sqlOutput)
	}
}

func TestPowerPlan(t *testing.T) {
	utils.WindowsOnly(t)

	command := fmt.Sprintf("powercfg /getactivescheme")
	output, err := utils.RunPowershellCmd(command)
	if err != nil {
		t.Fatalf("Unable to query active power plan: %v", err)
	}

	activePlan := strings.ToLower(strings.TrimSpace(output.Stdout))
	expectedPlan := "high performance"
	if !strings.Contains(activePlan, expectedPlan) {
		t.Fatalf("Active power plan is not %s: got %s", expectedPlan, activePlan)
	}
}

func TestRemoteConnectivity(t *testing.T) {
	utils.WindowsOnly(t)

	connectionCmd := `$SQLServer = (Invoke-RestMethod -Headers @{'Metadata-Flavor' = 'Google'} -Uri 'http://metadata.google.internal/computeMetadata/v1/instance/attributes/sqltarget')
	$SQLDBName = 'master'
	$DBUser = 'sa'
	$DBPass = 'ReMoTiNg@369!TEST#^*'

	$SqlConnection = New-Object System.Data.SqlClient.SqlConnection
	$SqlConnection.ConnectionString = "Server = $SQLServer; Database = $SQLDBName; User ID = $DBUser; Password = $DBPass; TrustServerCertificate = True"
	$SqlCmd = New-Object System.Data.SqlClient.SqlCommand
	$SqlCmd.CommandText = 'SELECT * FROM information_schema.tables'
	$SqlCmd.Connection = $SqlConnection

	$SqlAdapter = New-Object System.Data.SqlClient.SqlDataAdapter
	$SqlAdapter.SelectCommand = $SqlCmd

	$DataSet = New-Object System.Data.DataSet
	$result = $SqlAdapter.Fill($DataSet)
	$SqlConnection.Close()

	Write-Output $result`

	output, err := utils.RunPowershellCmd(connectionCmd)
	if err != nil {
		t.Fatalf("Unable to query server database: %v", err)
	}

	resultStr := strings.TrimSpace(output.Stdout)
	result, err := strconv.Atoi(resultStr)
	if 1 > result {
		t.Fatalf("Test output returned invalid rows; got %d, expected > 0", result)
	}
}

// TestSSMSVersion checks that the SSMS version is within the defined range to catch version updates
// or incorrect installation path.
func TestSSMSVersion(t *testing.T) {
	utils.WindowsOnly(t)
	minMajorVersion := 22
	maxMajorVersion := 23
	psCmd := `

# Expected SSMS 22 installation directory
 $ssms_path = "C:\Program Files\Microsoft SQL Server Management Studio*\Release\Common7\IDE\Ssms.exe"


# SSMS version range
$min_version = [Version]"22.0.0.0"
$max_version = [Version]"23.0.0.0"

$ssms_file = Get-ChildItem -Path $ssms_path -ErrorAction SilentlyContinue | Select-Object -First 1
if ($ssms_file) {
    $ssms_file_path = $ssms_file.FullName
    Write-Host "Found SSMS executable at: $ssms_file_path"
}
else {
    Write-Error "Error 1: SSMS.exe is not found in $ssms_path. Check if it's correctly installed, or if it's an older version installed at a different path, such as C:\Program Files (x86)\Microsoft SQL Server Management Studio*\Common7\IDE\Ssms.exe"
    exit 1
}

try {
    $product_version_string = $ssms_file.VersionInfo.ProductVersion
    $current_version = [Version]$product_version_string
}
catch {
    Write-Error "Could not retrieve or parse the version information from SSMS.exe at $ssms_file_path."
    exit 3 # Exit with a different error code for parsing failure
}

# Check if the version is within the defined range
if (($current_version -lt $min_version) -or ($current_version -ge $max_version)) {
    Write-Error "Error 2: SSMS version $current_version is not within the defined range ($min_version to < $max_version)."
    exit 2
}

Write-Host "Success: SSMS version $current_version is within the major version range [22, 23)."
exit 0
`
	output, err := utils.RunPowershellCmd(psCmd)
	if err != nil {
		switch output.Exitcode {
		case 1:
			t.Fatalf("SSMS version is not found: %v", output.Stderr)
		case 2:
			t.Fatalf("SSMS version is not within the defined range: %v", output.Stderr)
		default:
			t.Fatalf("Unable to query SSMS version: %v %v %v", output.Stdout, output.Stderr, err)
		}
	}
	fmt.Printf("SSMS version is %v within the defined range [%v, %v)", output.Stdout, minMajorVersion, maxMajorVersion)
}
