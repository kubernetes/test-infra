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

# This Dockerfile is used by ProwJobs that build docker images for the
# TF CRD in (https://github.com/jlewi/mlkube.io)

FROM golang:1.8.2
LABEL authors="Jeremy Lewi <jlewi@google.com>"

WORKDIR /workspace
RUN mkdir -p /workspace
ENV WORKSPACE=/workspace \
    TERM=xterm

# common util tools
# https://github.com/GoogleCloudPlatform/gsutil/issues/446 for python-openssl
RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    curl \
    file \
    rsync \
    ca-certificates \
    build-essential \
    openssh-client \
    git \
    pkg-config \
    zip \
    unzip \
    xz-utils \
    zlib1g-dev \
    python \
    python-openssl \
    python-pip \
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


# Install glide
RUN cd /tmp && \
    wget -O glide-v0.13.0-linux-amd64.tar.gz \
            https://github.com/Masterminds/glide/releases/download/v0.13.0/glide-v0.13.0-linux-amd64.tar.gz && \
    tar -xvf glide-v0.13.0-linux-amd64.tar.gz && \
    mv ./linux-amd64/glide /usr/local/bin/

ADD ["runner.py", \
    "/workspace/"]

ENTRYPOINT ["python", "/workspace/runner.py"]
