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
  [Parameter(Mandatory = $true)][string]$Modulus
)

$ErrorActionPreference = 'Stop'

$regPath = 'HKLM:\SOFTWARE\Google\ComputeEngine'
if (-not (Test-Path -LiteralPath $regPath)) {
  Write-Output 'NOT_FOUND'
  exit 0
}
$prop = Get-ItemProperty -LiteralPath $regPath -Name 'PublicKeys' -ErrorAction SilentlyContinue
if ($null -eq $prop -or $null -eq $prop.PublicKeys) {
  Write-Output 'NOT_FOUND'
  exit 0
}
foreach ($entry in @($prop.PublicKeys)) {
  if (([string]$entry).Contains($Modulus)) {
    Write-Output 'FOUND'
    exit 0
  }
}
Write-Output 'NOT_FOUND'
exit 0
