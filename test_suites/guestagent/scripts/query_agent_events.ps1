# Copyright 2026 Google LLC.
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

param (
  [ValidateSet('LatestRecordId', 'ListMessages')]
  [string]$Action = 'ListMessages',
  [long]$MinRecordID = 0,
  [int]$EventID = 882,
  [string]$Provider = ''
)

$ErrorActionPreference = 'Stop'

if ($Action -eq 'LatestRecordId') {
  $ev = Get-WinEvent -LogName Application -MaxEvents 1 -ErrorAction SilentlyContinue
  if ($null -eq $ev) {
    Write-Output '0'
  }
  else {
    Write-Output $ev.RecordId
  }
  exit 0
}

$escapedProvider = $Provider -replace '&', '&amp;' -replace '<', '&lt;' -replace '>', '&gt;' -replace "'", '&apos;'
$xpath = "*[System[EventRecordID > $MinRecordID and EventID = $EventID and Provider[@Name='$escapedProvider']]]"
$events = Get-WinEvent -LogName Application -FilterXPath $xpath -Oldest -ErrorAction SilentlyContinue
foreach ($ev in $events) {
  Write-Output ($ev.Message -replace '\r?\n', ' ')
}
exit 0
