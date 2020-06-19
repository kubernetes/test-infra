# Copyright 2018 The Kubernetes Authors.
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

ARG GO_VERSION

# Includes bash, docker, and gcloud
FROM golang:${GO_VERSION}-alpine

# add env we can debug with the image name:tag
ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

# Install gcloud, docker, and bash
ENV PATH=/google-cloud-sdk/bin:/workspace:${PATH} \
    CLOUDSDK_CORE_DISABLE_PROMPTS=1

WORKDIR /workspace

RUN echo http://dl-cdn.alpinelinux.org/alpine/latest-stable/community >> /etc/apk/repositories && \
    apk --no-cache add curl python py-crcmod bash libc6-compat openssh-client git gnupg docker-cli make && \
    curl -O https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz && \
    tar xzf google-cloud-sdk.tar.gz -C / && \
    rm google-cloud-sdk.tar.gz && \
    /google-cloud-sdk/install.sh \
        --disable-installation-options \
        --bash-completion=false \
        --path-update=false \
        --usage-reporting=false && \
    gcloud info > /workspace/gcloud-info.txt

# Install docker Buildx for fast docker builds
ENV HOME=/root
RUN mkdir -p  $HOME/.docker/cli-plugins \
    && curl -fsSL "https://github.com/docker/buildx/releases/download/v0.4.1/buildx-v0.4.1.linux-amd64" --output $HOME/.docker/cli-plugins/docker-buildx \
    && chmod a+x $HOME/.docker/cli-plugins/docker-buildx

ENTRYPOINT ["/bin/bash"]
