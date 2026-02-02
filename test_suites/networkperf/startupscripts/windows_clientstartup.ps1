# Copyright 2024 Google LLC.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

$hostname = [System.Net.Dns]::GetHostName().Split('.')[0]

try {
  $numtests = Invoke-RestMethod -Headers @{'Metadata-Flavor'='Google'} -Uri http://metadata.google.internal/computeMetadata/v1/instance/attributes/num-parallel-tests -UseBasicParsing
}
catch {
  Write-Host "Failed to get num-parallel-tests from metadata: $_"
  exit 1
}
$baseport = 5001
$sleepduration = 5
$conn_timeout_sec = 300
$conn_retries = $conn_timeout_sec / $sleepduration

# Test whether the servers are up.
Write-Host "Checking if $numtests servers are up"
for ($i = 0; $i -lt $numtests; $i++) {
    $iperftarget = Invoke-RestMethod -Uri "http://metadata.google.internal/computeMetadata/v1/instance/attributes/iperftarget-$i" -Headers @{'Metadata-Flavor'='Google'} -ErrorAction Stop -UseBasicParsing
    $port = $baseport + $i
    Write-Host "Checking connection to $iperftarget`:$port"
    $connected = $false
    for ($j = 0; $j -lt $conn_retries; $j++) {
        $test = Test-NetConnection -ComputerName $iperftarget -Port $port -ErrorAction SilentlyContinue
        if ($test.TcpTestSucceeded) {
            Write-Host "Connection to $iperftarget`:$port succeeded."
            $connected = $true
            break
        }
        Write-Host "Connection to $iperftarget`:$port failed. Checking again in $sleepduration s..."
        Start-Sleep -s $sleepduration
    }
    if (-not $connected) {
        Write-Host "FATAL: Could not connect to $iperftarget`:$port after $conn_timeout_sec seconds."
        exit 1
    }
}

Start-Sleep -s $sleepduration

# Perform the test, and upload results.
$jobs = @()
for ($i = 0; $i -lt $numtests; $i++) {
    $iperftarget = Invoke-RestMethod -Uri "http://metadata.google.internal/computeMetadata/v1/instance/attributes/iperftarget-$i" -Headers @{'Metadata-Flavor'='Google'} -ErrorAction Stop -UseBasicParsing
    $port = $baseport + $i
    $outfile="C:\iperf\$hostname-$i.txt"
    Write-Host "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss'): Running iperf client $i with target $iperftarget`:$port"
    $jobs += Start-Job -ScriptBlock {
        & "$using:exepath\iperf.exe" -c $using:iperftarget -p $using:port -t 30 -P 16 *> $using:outfile
    }
}

Write-Host 'Waiting for tests to complete...'
Wait-Job -Job $jobs
Get-Job | Receive-Job # Gets job output/errors if any.
Write-Host 'All tests completed.'

for ($i = 0; $i -lt $numtests; $i++) {
    $outfile="C:\iperf\$hostname-$i.txt"
    $metadata="http://metadata.google.internal/computeMetadata/v1/instance/guest-attributes/testing/results-$i"
    Write-Host "Uploading results from $outfile to $metadata"
    $results = (Get-Content -Path $outfile | Select-String -Pattern 'SUM' | Select-Object -Last 1) -replace '\s+',' '
    for ($j = 0; $j -lt 3; $j++) {
        Start-Sleep -Seconds $j
        try {
            $results | Invoke-RestMethod -Method 'Put' -Uri $metadata -Headers @{'Metadata-Flavor'='Google'} -ErrorAction Stop -UseBasicParsing
            Write-Host "Successfully uploaded results for test $i"
            break
        }
        catch {
            Write-Host "Attempt $j failed to upload results for test $i -- $($PSItem.Exception.Message)"
        }
    }
}
