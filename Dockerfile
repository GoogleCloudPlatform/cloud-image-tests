# Copyright 2021 Google LLC
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

FROM golang:1.21-alpine as builder

WORKDIR /build
COPY . .
RUN mkdir /out

ENV GOOS linux
ENV GO111MODULE on

RUN ./local_build.sh -o /out

FROM alpine:latest
RUN apk add --no-cache openssh-keygen
RUN apk update && apk upgrade
COPY --from=builder /out/* /

ENTRYPOINT ["/manager"]
