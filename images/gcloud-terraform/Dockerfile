# Copyright 2021 The Kubernetes Authors.
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

FROM gcr.io/k8s-prow/alpine:v20210618-2814345

# add env we can debug with the image name:tag
ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

# Install utilities and dependencies
#
# Terraform requires `git` to be on the PATH for downloading modules from
# GitHub.
RUN apk update && apk add --no-cache \
    bash \
    git \
    python3 \
    make \
    rsync \
    wget \
    curl && \
    rm -rf /var/lib/apt/lists/*

# Install gcloud
ENV PATH=/google-cloud-sdk/bin:/workspace:${PATH} \
    CLOUDSDK_CORE_DISABLE_PROMPTS=1
WORKDIR /workspace

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

# Install terraform
# Doc: https://pkgs.alpinelinux.org/packages?name=terraform&branch=v3.14&arch=x86_64
RUN apk add --no-cache terraform==0.14.9-r4 --repository=http://dl-cdn.alpinelinux.org/alpine/v3.14/main

ENTRYPOINT ["/bin/bash"]
