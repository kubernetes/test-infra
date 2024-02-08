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

FROM multiarch/qemu-user-static:7.2.0-1 as qemu-image

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
    apk --no-cache add curl python3 py-crcmod bash libc6-compat openssh-client git gnupg docker-cli make

RUN curl -fsSLO https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz
RUN tar xzf google-cloud-sdk.tar.gz -C /
RUN rm google-cloud-sdk.tar.gz
RUN gcloud config set core/disable_usage_reporting true && \
    gcloud config set component_manager/disable_update_check true && \
    gcloud config set metrics/environment github_docker_image && \
    gcloud components install alpha beta gke-gcloud-auth-plugin gsutil && \
    gcloud --version && \
    gcloud info > /workspace/gcloud-info.txt

# Default home for cloudbuild jobs is /builder/home
RUN mkdir -p /builder/home

# Copy qemu static binaries.
COPY --from=qemu-image /usr/bin /qemu-bin
RUN cp /qemu-bin/qemu-* /usr/bin/ && \
    rm -rf /qemu-bin
COPY --from=qemu-image /qemu-binfmt-conf.sh /qemu-binfmt-conf.sh
COPY --from=qemu-image /register /register
ADD ./buildx-entrypoint.sh /buildx-entrypoint

RUN apk add qemu

# Install buildx system-wide for fast docker builds
COPY --from=docker/buildx-bin:0.12.1 /buildx /usr/libexec/docker/cli-plugins/docker-buildx

ENTRYPOINT ["/bin/bash"]
