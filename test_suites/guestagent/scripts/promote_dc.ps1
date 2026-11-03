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
  [Parameter(Mandatory = $true)][string]$AdminUser,
  [Parameter(Mandatory = $true)][string]$AdminPassword,
  [Parameter(Mandatory = $true)][string]$DomainName,
  [Parameter(Mandatory = $true)][string]$DomainNetbiosName,
  [Parameter(Mandatory = $true)][string]$SafeModePassword
)

$ErrorActionPreference = 'Stop'

# Sysprepped GCE Windows Server images start with local Administrator disabled
# and a blank password, which Install-ADDSForest rejects because local
# Administrator becomes the domain Administrator account.
Add-Type -AssemblyName System.DirectoryServices.AccountManagement
$ctx = New-Object System.DirectoryServices.AccountManagement.PrincipalContext(
  [System.DirectoryServices.AccountManagement.ContextType]::Machine
)
try {
  $admin = [System.DirectoryServices.AccountManagement.UserPrincipal]::FindByIdentity(
    $ctx,
    [System.DirectoryServices.AccountManagement.IdentityType]::SamAccountName,
    $AdminUser
  )
  if ($null -eq $admin) {
    throw "Local $AdminUser account not found"
  }
  $admin.SetPassword($AdminPassword)
  $admin.Enabled = $true
  $admin.Save()
  if (-not $ctx.ValidateCredentials($AdminUser, $AdminPassword)) {
    throw "Failed to validate initial local $AdminUser password before DC promotion"
  }
} finally {
  $ctx.Dispose()
}

$featureResult = Install-WindowsFeature -Name AD-Domain-Services,DNS -IncludeManagementTools
if (-not $featureResult.Success) {
  throw "Install-WindowsFeature failed: $($featureResult | Out-String)"
}

Import-Module ADDSDeployment
$dsrm = ConvertTo-SecureString $SafeModePassword -AsPlainText -Force
$forestResult = Install-ADDSForest `
  -DomainName $DomainName `
  -DomainNetbiosName $DomainNetbiosName `
  -SafeModeAdministratorPassword $dsrm `
  -InstallDns:$true `
  -NoRebootOnCompletion:$true `
  -Force:$true
if ($null -eq $forestResult) {
  throw 'Install-ADDSForest returned null result'
}
if ($forestResult.Status -ne 'Success') {
  throw "Install-ADDSForest returned status $($forestResult.Status): $($forestResult.ContextMessage)"
}
exit 0
