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

$AUTH_SCRIPT = 'https://storage.googleapis.com/windows-utils/change_auth.sql'
Invoke-WebRequest -Uri $AUTH_SCRIPT -OutFile c:\\change_auth.sql

try {
    sqlcmd -S localhost -i c:\change_auth.sql
} catch {
    Write-Host "Failed to set auth config: $_"
} finally {
    Restart-Service MSSQLSERVER
    Write-Host "auth config updated"
}
