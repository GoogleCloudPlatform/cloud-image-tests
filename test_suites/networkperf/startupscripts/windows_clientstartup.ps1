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

$outfile="C:\iperf\iperfoutput.txt"
$metadata="http://metadata.google.internal/computeMetadata/v1/instance/guest-attributes/testing/results"
$iperftarget=Invoke-RestMethod -Uri "http://metadata.google.internal/computeMetadata/v1/instance/attributes/iperftarget" -Header @{"Metadata-Flavor" = "Google"} -UseBasicParsing

# Test whether the server is up.
$conntest="tmp_connection_test.txt"
Test-NetConnection -ComputerName $iperftarget -P 5001 > $conntest
while ( (Get-Content -Path $conntest | Select-String -Pattern 'TcpTestSucceeded') -like '*False*')
{
        Write-Host "Connection to server failed. Checking again in 5s..."
        Start-Sleep -s 5
        Test-NetConnection -ComputerName $iperftarget -P 5001 > $conntest
}
Start-Sleep -s 5

# Perform the test, and upload results.
./iperf -c $iperftarget -t 30 -P 16 2>&1 | Tee-Object -FilePath $outfile

for (($i = 0); $i -lt 3; $i++)
{
  Start-Sleep -Seconds $i
  (Get-Content -Path $outfile | Select-String -Pattern 'SUM') -replace "\s+"," " | Invoke-RestMethod -Method "Put" -Uri $metadata -Header @{"Metadata-Flavor" = "Google"} -ContentType "application/json; charset=utf-8" -UseBasicParsing
  if ($?) {
    break
  }
}
