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
  [Parameter(Mandatory = $true)]
  [ValidateSet('UserExists', 'InAdminGroup', 'Validate', 'EnsurePassword')]
  [string]$Action,
  [Parameter(Mandatory = $true)][string]$DomainDN,
  [Parameter(Mandatory = $true)][string]$SamAccountName,
  [string]$Password = '',
  [string]$AdminGroupSID = 'S-1-5-32-544'
)

$ErrorActionPreference = 'Stop'

# Bind directly to $env:COMPUTERNAME so AD queries do not depend on NIC DNS
# resolving the AD domain if DHCP renews and resets NIC DNS to 169.254.169.254.
Add-Type -AssemblyName System.DirectoryServices.AccountManagement
$ctx = New-Object System.DirectoryServices.AccountManagement.PrincipalContext(
  [System.DirectoryServices.AccountManagement.ContextType]::Domain,
  $env:COMPUTERNAME,
  $DomainDN
)

try {
  switch ($Action) {
    'UserExists' {
      $u = [System.DirectoryServices.AccountManagement.UserPrincipal]::FindByIdentity(
        $ctx,
        [System.DirectoryServices.AccountManagement.IdentityType]::SamAccountName,
        $SamAccountName
      )
      if ($null -ne $u) {
        Write-Output 'FOUND'
      }
      else {
        Write-Output 'NOT_FOUND'
      }
    }
    'InAdminGroup' {
      $u = [System.DirectoryServices.AccountManagement.UserPrincipal]::FindByIdentity(
        $ctx,
        [System.DirectoryServices.AccountManagement.IdentityType]::SamAccountName,
        $SamAccountName
      )
      if ($null -eq $u) {
        Write-Output 'NOT_MEMBER'
      }
      else {
        $grp = [ADSI]"LDAP://$($env:COMPUTERNAME)/<SID=$AdminGroupSID>"
        if ($grp.IsMember("LDAP://$($env:COMPUTERNAME)/$($u.DistinguishedName)")) {
          Write-Output 'MEMBER'
        }
        else {
          Write-Output 'NOT_MEMBER'
        }
      }
    }
    'Validate' {
      if ([string]::IsNullOrEmpty($Password)) {
        throw "Password must not be empty for Action=$Action"
      }
      $valid = $ctx.ValidateCredentials(
        $SamAccountName,
        $Password,
        [System.DirectoryServices.AccountManagement.ContextOptions]::Negotiate
      )
      if ($valid) {
        Write-Output 'VALID'
      }
      else {
        Write-Output 'INVALID'
      }
    }
    'EnsurePassword' {
      if ([string]::IsNullOrEmpty($Password)) {
        throw "Password must not be empty for Action=$Action"
      }
      $valid = $ctx.ValidateCredentials(
        $SamAccountName,
        $Password,
        [System.DirectoryServices.AccountManagement.ContextOptions]::Negotiate
      )
      if (-not $valid) {
        $u = [System.DirectoryServices.AccountManagement.UserPrincipal]::FindByIdentity(
          $ctx,
          [System.DirectoryServices.AccountManagement.IdentityType]::SamAccountName,
          $SamAccountName
        )
        if ($null -eq $u) {
          throw "Domain account $SamAccountName not found"
        }
        $u.SetPassword($Password)
        $u.Enabled = $true
        $u.Save()
      }
    }
    default {
      throw "Unsupported Action: $Action"
    }
  }
} finally {
  $ctx.Dispose()
}
exit 0
