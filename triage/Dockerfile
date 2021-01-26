# Copyright 2020 The Kubernetes Authors.
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

# This Dockerfile uses multi-stage builds. The first stage builds Triage and gathers the Google
# Cloud SDK, and the resulting files are then used in the second stage.

# Stage 1
FROM golang:1.13 AS build

COPY . /test-infra/

# Build Triage
RUN cd /test-infra/triage/ && \
    env GOOS=linux \
        CGO_ENABLED=0 \
        GO111MODULE=on \
        GOPROXY=https://proxy.golang.org \
        GOSUMDB=sum.golang.org \
        go build -v -o triage ./main.go
# Get the Google Cloud SDK
RUN curl -o installer https://sdk.cloud.google.com && \
    bash installer --disable-prompts --install-dir=/ && \
    rm installer


# Stage 2
FROM alpine:3.12.0

# Google Cloud SDK requires python 2.7.9+ (https://cloud.google.com/python/setup#installing_the_cloud_sdk).
# update_summaries.sh requires bash.
RUN apk add --no-cache python2 bash

# Copy over the triage binary
COPY --from=build /test-infra/triage/triage /

# Copy the Google Cloud SDK and link the binaries
COPY --from=build /google-cloud-sdk/ /google-cloud-sdk/
RUN ln -s /google-cloud-sdk/bin/* /bin/

# Copy over the shell script that orchestrates everything
COPY ./triage/update_summaries.sh /

# Point GOOGLE_APPLICATION_CREDENTIALS at a serviceaccount.json with the necessary permissions.
ENTRYPOINT ["timeout", "10800", "/update_summaries.sh"]
