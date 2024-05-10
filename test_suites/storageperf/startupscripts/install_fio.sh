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

if [[ -f /usr/bin/apt ]]; then
	apt -y update && apt -y install fio
elif [[ -f /bin/dnf ]]; then 
	dnf -y install fio
elif [[ -f /bin/yum ]]; then
	yum -y install fio
elif [[ -f /usr/bin/zypper ]]; then
	zypper --non-interactive install fio
else 
	echo "No package managers found to install fio"
fi

errorcode=$?
if [[ errorcode != 0 ]]; then
	echo "Error running install fio command: code $errorcode"
