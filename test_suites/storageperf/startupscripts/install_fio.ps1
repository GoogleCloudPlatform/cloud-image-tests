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

# If testing windows performance locally, you may need to put a copy of fio.exe in a local bucket. Ensure the service account has permissions to access the bucket, and change the gcs path here.
# TODO: consider other methods of installing fio on windows.
gcloud storage cp gs://gce-image-build-resources/windows/fio.exe 'C:\\fio.exe'
