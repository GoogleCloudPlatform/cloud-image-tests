#!/bin/bash
# Test that the CIT manager can generate a workflow for a linux amd64 image, a
# linux arm64 image, and a windows image. This script is configured with the
# following environment variables with the following defaults:
#
# linux_image_amd64 = projects/debian-cloud/global/images/family/debian-12
#   The linux amd64 image to generate a workflow for
# linux_image_arm64 = projects/debian-cloud/global/images/family/debian-12-arm64
#   The linux arm64 image to generate a workflow for
# windows_image_amd64 = projects/windows-cloud/global/images/family/windows-2022
#   The windows amd64 image to generate a workflow for
# project = (none)
#   The project to use for workflow generation and image lookup
# cit_dir = $(dirname $0)
#   The directory containing CIT binaries. Only the manager is required.

set -xe
linux_image_amd64=${linux_image_amd64:="projects/debian-cloud/global/images/family/debian-12"}
linux_image_arm64=${linux_image_arm64:="projects/debian-cloud/global/images/family/debian-12-arm64"}
windows_image_amd64=${windows_image_amd64:="projects/windows-cloud/global/images/family/windows-2022"}
project=${project:=""}
cit_dir=${cit_dir:="$(dirname $0)"}

$cit_dir/manager -print -images $linux_image_amd64,$linux_image_arm64,$windows_image_amd64 -project $project > /dev/null
