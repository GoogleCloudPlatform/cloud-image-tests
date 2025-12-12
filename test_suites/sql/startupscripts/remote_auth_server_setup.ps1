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

$sqlservice = Get-Service 'MSSQLSERVER'
$sqlservice.WaitForStatus('Running', '00:10:00')
Write-Host 'MSSQLSERVER is now running.'

$cn = [ADSI]"WinNT://$env:COMPUTERNAME"
$user = $cn.Create('User', 'SqlTests')
$user.SetPassword('remoting@123')
$user.SetInfo()
$user.description = 'Admin user to install new software'
$user.SetInfo()
$group = [ADSI]"WinNT://$env:COMPUTERNAME/Administrators"
$group.Add($user.Path)

$ErrorActionPreference = 'Stop'

# SQL commands embedded as a here-string
$sqlCommands = @"
USE [master];
GO

-- Set LoginMode to 2 (Mixed Mode) in the correct location
  PRINT 'Writing to registry...';
  EXEC xp_instance_regwrite N'HKEY_LOCAL_MACHINE', N'Software\Microsoft\MSSQLServer\MSSQLServer', N'LoginMode', REG_DWORD, 2;
  PRINT 'Registry write command executed.';
GO

-- Configure 'sa' account
PRINT 'Altering LOGIN sa WITH PASSWORD...';
ALTER LOGIN sa WITH
  PASSWORD = 'ReMoTiNg@369!TEST#^*';
--  CHECK_POLICY = OFF;
GO
ALTER LOGIN sa ENABLE;
GO
"@

try {
    sqlcmd -C -S localhost -Q $sqlCommands -b
} catch {
    Write-Host "Failed to set auth config: $_"
} finally {
    Restart-Service MSSQLSERVER
    Write-Host "Server authentication config update process finished."
}
