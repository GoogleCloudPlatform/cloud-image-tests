#!/bin/bash
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

# usage: local_build.sh -o $outspath -i $imagetestroot -s $suites_to_build -j $jobs
# the output path of the test files
outpath=.
# the suites to build, space separated. all suites are built by default
suites=*
# the path to the imagetest folder, default value assumes this script is run from the imagetest folder. If set, the commands cd $imagetestroot/cmd and cd $imagetestroot/test_suites should succeed.
imagetestroot=.
# max suites to build concurrently. 0 means auto: max(1, (nproc-1)/3),
# on the theory that each `go test -c` invocation spawns ~3 hot worker
# processes (compile, vet, link), so K*3 <= cores-1. Pass -j N to override.
jobs=0


while getopts "o:s:i:j:" arg; do
  case $arg in
    o) outpath=$OPTARG;;
    s) suites=$OPTARG;;
    i) imagetestroot=$OPTARG;;
    j) jobs=$OPTARG;;
    *) echo "unknown arg"
  esac
done

if [ "$jobs" -eq 0 ]; then
  jobs=$(( ($(nproc) - 1) / 3 ))
  [ $jobs -lt 1 ] && jobs=1
fi


export CGO_ENABLED=0
echo "outspath is $outpath"
echo "suites being built are $suites"
echo "imagetestroot is $imagetestroot"

cd $imagetestroot
go mod download
go build -o $outpath/wrapper.amd64 ./cmd/wrapper/main.go
GOARCH=arm64 go build -o $outpath/wrapper.arm64 ./cmd/wrapper/main.go || exit 1
GOOS=windows GOARCH=amd64 go build -o $outpath/wrapp64.exe ./cmd/wrapper/main.go || exit 1
GOOS=windows GOARCH=386 go build -o $outpath/wrapp32.exe ./cmd/wrapper/main.go || exit 1
go build -o $outpath/manager ./cmd/manager/main.go || exit 1


# Build one suite (all four arch variants). Run in its own subshell so cd
# stays scoped and so multiple suites can run concurrently.
build_suite() {
  local suite=$1
  local outpath=$2
  local root=$3
  cd "$root/test_suites/$suite" || return 1
  echo "building suite $suite"
  go test -c -tags cit || return 1
  ./"${suite}.test" -test.list '.*' > "$outpath/${suite}_tests.txt" || return 1
  mv "${suite}.test" "$outpath/${suite}.amd64.test" || return 1
  GOARCH=arm64 go test -c -tags cit || return 1
  mv "${suite}.test" "$outpath/${suite}.arm64.test" || return 1
  GOOS=windows GOARCH=amd64 go test -c -tags cit || return 1
  if [ -f "${suite}.test.exe" ]; then mv "${suite}.test.exe" "$outpath/${suite}64.exe" || return 1; fi
  GOOS=windows GOARCH=386 go test -c -tags cit || return 1
  if [ -f "${suite}.test.exe" ]; then mv "${suite}.test.exe" "$outpath/${suite}32.exe" || return 1; fi
}

abs_outpath=$(cd "$outpath" && pwd)
abs_root=$(cd "$imagetestroot" && pwd)

# FIFO semaphore: $jobs slots in a named pipe. Each suite reads a token
# before starting and writes one back when done.
slotfifo=$(mktemp -u)
mkfifo "$slotfifo"
exec 9<>"$slotfifo"
rm "$slotfifo"
for i in $(seq 1 $jobs); do echo >&9; done

cd "$abs_root/test_suites"
pids=()
names=()
for suite in $suites; do
  [[ -d "$suite" ]] || continue
  read -u 9 # wait for a slot
  (
    build_suite "$suite" "$abs_outpath" "$abs_root"
    rc=$?
    echo >&9 # release slot regardless of outcome
    exit $rc
  ) &
  pids+=($!)
  names+=("$suite")
done

fail=0
for i in "${!pids[@]}"; do
  if ! wait "${pids[$i]}"; then
    echo "[!] suite ${names[$i]} failed"
    fail=1
  fi
done
exec 9>&-
[[ $fail -eq 0 ]] || exit 1
