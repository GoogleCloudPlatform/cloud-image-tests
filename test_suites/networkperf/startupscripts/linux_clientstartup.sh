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

# This script installs iperf on a VM and attempts to connect to an iperf
# server to test the network bandwidth between the two VMs.

hostname=$(curl http://metadata.google.internal/computeMetadata/v1/instance/hostname -H "Metadata-Flavor: Google" | cut -d"." -f1)
numtests=$(curl http://metadata.google.internal/computeMetadata/v1/instance/attributes/num-parallel-tests -H "Metadata-Flavor: Google")
sleepduration=5
timeout=0

function outfile_name() {
  echo "$hostname-$1.txt"
}

# Ensure the server is up and running.
echo "Checking if server is up"
for i in $(seq 0 $((numtests-1))); do
  port=$((5001+i))
  iperftarget=$(curl http://metadata.google.internal/computeMetadata/v1/instance/attributes/iperftarget-$i -H "Metadata-Flavor: Google")
  timeout 2 nc -v -w 1 "$iperftarget" "$port" &> /tmp/nc_iperf
  until [[ $(< /tmp/nc_iperf) == *"succeeded"* || $(< /tmp/nc_iperf) == *"Connected"* || "$timeout" -ge "$maxtimeout" ]]; do
    cat /tmp/nc_iperf
    echo Failed to connect to server. Trying again in 5s
    sleep "$sleepduration"
    timeout=$((timeout+sleepduration))

    # timeout ensures the command stops. On some versions of netcat,
    # the -w flag seems nonfunctional. This is the workaround.
    timeout 2 nc -v -w 1 "$iperftarget" "$port" &> /tmp/nc_iperf
  done
done

if [[ $timeout -ge $maxtimeout ]]; then
  exit 1
fi
sleep "$sleepduration"

# Run iperf in parallel.
for i in $(seq 0 $((numtests-1))); do
  port=$((5001+i))
  iperftarget=$(curl http://metadata.google.internal/computeMetadata/v1/instance/attributes/iperftarget-$i -H "Metadata-Flavor: Google")

  echo "$(date +"%Y-%m-%d %T"): Running iperf client with target $iperftarget"
  (iperf -t 30 -c "$iperftarget" -P $parallelcount -p "$port" 2>&1 | tee "$(outfile_name $i)") &
done

wait

# Upload results.
for i in $(seq 0 $((numtests-1))); do
  results=$(cat "$(outfile_name $i)" | grep SUM | tr -s ' ' 2>&1)
  for retry_backoff in $(seq 0 2); do
    sleep $retry_backoff
    curl -X PUT --data "$results" http://metadata.google.internal/computeMetadata/v1/instance/guest-attributes/testing/results-$i -H "Metadata-Flavor: Google" && break
  done
done
