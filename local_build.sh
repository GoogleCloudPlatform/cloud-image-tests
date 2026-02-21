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

# usage: local_build.sh -o $outspath -i $imagetestroot -s $suites_to_build
# the output path of the test files
outpath=.
# the suites to build, space separated. all suites are built by default
suites=*
# the path to the imagetest folder, default value assumes this script is run from the imagetest folder. If set, the commands cd $imagetestroot/cmd and cd $imagetestroot/test_suites should succeed.
imagetestroot=.
# number of suites to build simultaneously
threads=1

while getopts "o:s:i:j:" arg; do
  case $arg in
    o)
      # Check if the path is absolute (starts with a '/')
      if [[ "$OPTARG" =~ ^/ ]]; then
        outpath="$OPTARG"
      else
        # If it's a relative path, prepend the current working directory
        outpath="$(pwd)/$OPTARG"
      fi
      ;;
    s) suites=$OPTARG;;
    i) imagetestroot=$OPTARG;;
    j) threads=$OPTARG;;
    *) echo "unknown arg"
  esac
done


export CGO_ENABLED=0
echo "outpath is $outpath"
echo "suites being built are $suites"
echo "imagetestroot is $imagetestroot"
echo "building $threads suite(s) simultaneously"

cd $imagetestroot
go mod download
go build -o $outpath/wrapper.amd64 ./cmd/wrapper/main.go
GOARCH=arm64 go build -o $outpath/wrapper.arm64 ./cmd/wrapper/main.go || exit 1
GOOS=windows GOARCH=amd64 go build -o $outpath/wrapp64.exe ./cmd/wrapper/main.go || exit 1
GOOS=windows GOARCH=386 go build -o $outpath/wrapp32.exe ./cmd/wrapper/main.go || exit 1
go build -o $outpath/manager ./cmd/manager/main.go || exit 1


cd test_suites
JOBS=()     # Array to track PIDs of build jobs

for suite in $suites; do
  [[ -d $suite ]] || continue
  (
    cd $suite
    echo "building suite $suite"
    go test -c -tags cit || exit 1
    ./"${suite}.test" -test.list '.*' > $outpath/"${suite}_tests.txt" || exit 1
    mv "${suite}.test" $outpath/"${suite}.amd64.test" || exit 1
    GOARCH=arm64 go test -c -tags cit || exit 1
    mv "${suite}.test" "$outpath/${suite}.arm64.test" || exit 1
    GOOS=windows GOARCH=amd64 go test -c -tags cit || exit 1
    if [ -f "${suite}.test.exe" ]; then mv "${suite}.test.exe" "$outpath/${suite}64.exe" || exit 1; fi;
    GOOS=windows GOARCH=386 go test -c -tags cit || exit 1
    if [ -f "${suite}.test.exe" ]; then mv "${suite}.test.exe" "$outpath/${suite}32.exe" || exit 1; fi;
  ) &

  # Add the PID of the last build job to the array
  JOBS+=($!)

  # Check if the number of running builds has reached the limit
  if (( ${#JOBS[@]} >= $threads )); then
    # Wait for any one of the current builds
    wait -n
    # Rebuild the array of active build PIDs
    JOBS=( $(jobs -p) )
  fi
done

# Wait for all builds to complete
wait
echo "All selected test suites have been built."
