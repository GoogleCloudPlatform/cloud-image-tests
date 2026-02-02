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

Write-Host 'Starting iperf server'
try {
  $numtests = Invoke-RestMethod -Headers @{'Metadata-Flavor'='Google'} -Uri http://metadata.google.internal/computeMetadata/v1/instance/attributes/num-parallel-tests
}
catch {
  Write-Host "Failed to get num-parallel-tests from metadata: $_"
  exit 1
}

for ($i = 0; $i -lt $numtests; $i++) {
  $port = 5001 + $i
  Start-Process .\iperf.exe -ArgumentList "-s -p $port" -WindowStyle hidden
}
