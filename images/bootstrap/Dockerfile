# Copyright 2017 The Kubernetes Authors.
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

# Includes basic workspace setup, with gcloud and a bootstrap runner

FROM debian:jessie
LABEL authors="Sen Lu <senlu@google.com>"

WORKDIR /workspace
RUN mkdir -p /workspace
ENV WORKSPACE=/workspace \
    TERM=xterm

# common util tools
# https://github.com/GoogleCloudPlatform/gsutil/issues/446 for python-openssl
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    curl \
    file \
    git \
    jq \
    openssh-client \
    pkg-config \
    python \
    python-openssl \
    python-pip \
    rsync \
    unzip \
    wget \
    xz-utils \
    zip \
    zlib1g-dev \
    && apt-get clean

# Install gcloud

ENV PATH=/google-cloud-sdk/bin:/workspace:${PATH} \
    CLOUDSDK_CORE_DISABLE_PROMPTS=1

RUN wget -q https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz && \
    tar xzf google-cloud-sdk.tar.gz -C / && \
    rm google-cloud-sdk.tar.gz && \
    /google-cloud-sdk/install.sh \
        --disable-installation-options \
        --bash-completion=false \
        --path-update=false \
        --usage-reporting=false && \
    gcloud components install alpha beta kubectl && \
    gcloud info | tee /workspace/gcloud-info.txt

# Install docker
# Note: 1.11+ changes the tarball format
RUN curl -L "https://get.docker.com/builds/Linux/x86_64/docker-1.9.1.tgz" \
    | tar -C /usr/bin -xvzf- --strip-components=3 usr/local/bin/docker


ADD ["runner", \
    "/workspace/"]

ENTRYPOINT ["/workspace/runner"]
