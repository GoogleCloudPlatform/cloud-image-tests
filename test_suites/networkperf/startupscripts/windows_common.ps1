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

$maxtimeout=300

$iperfurl="https://iperf.fr/download/windows/iperf-2.0.9-win64.zip"
$iperfzippath="iperf.zip"
$zipdir="C:\iperf"
$exepath="C:\iperf\iperf-2.0.9-win64"

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
Invoke-WebRequest -Uri $iperfurl -OutFile $iperfzippath
Expand-Archive -Path $iperfzippath -DestinationPath $zipdir
New-NetFirewallRule -DisplayName 'allow-iperf' -Direction Inbound -LocalPort 5001-5010 -Protocol TCP -Action Allow

Set-Location $exepath
