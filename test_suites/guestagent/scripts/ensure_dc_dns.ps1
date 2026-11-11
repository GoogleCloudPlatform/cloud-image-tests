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
  [ValidateSet('true', 'false')]
  [string]$RequireRunningDns = 'false'
)

$ErrorActionPreference = 'Stop'

# Install-ADDSForest sets NIC DNS to 127.0.0.1. Configure both the Windows DNS
# Server forwarder and NIC DNS fallback to 169.254.169.254 so GCS and metadata
# hostnames resolve before and during AD DNS startup across reboots.
Get-NetAdapter | Where-Object { $_.Status -eq 'Up' } | ForEach-Object {
  Set-DnsClientServerAddress -InterfaceIndex $_.InterfaceIndex -ServerAddresses @('127.0.0.1', '169.254.169.254')
}

$dnsSvc = Get-Service -Name 'DNS' -ErrorAction SilentlyContinue
if ($RequireRunningDns -eq 'true' -and $null -eq $dnsSvc) {
  throw 'DNS service is not installed'
}
if ($null -ne $dnsSvc) {
  if ($dnsSvc.Status -ne 'Running') {
    if ($RequireRunningDns -eq 'true') {
      Start-Service -Name 'DNS'
    }
    else {
      Start-Service -Name 'DNS' -ErrorAction SilentlyContinue
    }
    $dnsSvc.Refresh()
  }
  if ($RequireRunningDns -eq 'true' -and $dnsSvc.Status -ne 'Running') {
    throw "DNS service status is $($dnsSvc.Status), want Running"
  }
  if ($dnsSvc.Status -eq 'Running') {
    try {
      Import-Module DnsServer -ErrorAction SilentlyContinue
      if (Get-Command Set-DnsServerForwarder -ErrorAction SilentlyContinue) {
        Set-DnsServerForwarder -IPAddress '169.254.169.254' -UseRootHint $false
      }
      else {
        dnscmd.exe 127.0.0.1 /ResetForwarders 169.254.169.254 /Slave | Out-Null
        if ($LASTEXITCODE -ne 0) {
          throw "dnscmd.exe /ResetForwarders failed with exit code $LASTEXITCODE"
        }
      }
    }
    catch {
      if ($RequireRunningDns -eq 'true') {
        throw
      }
    }
  }
}

[void][System.Net.Dns]::GetHostAddresses('metadata.google.internal')
[void][System.Net.Dns]::GetHostAddresses('storage.googleapis.com')
exit 0
