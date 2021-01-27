# Copyright 2016 The Kubernetes Authors.
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

# Includes git, gcloud, and bazel.
ARG OLD_BAZEL_VERSION
FROM launcher.gcr.io/google/bazel:${OLD_BAZEL_VERSION} as old
FROM ubuntu:18.04

# add env we can debug with the image name:tag
ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

ARG BAZEL_VERSION

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    ca-certificates \
    file \
    git \
    openssh-client \
    python2.7 \
    python3 \
    python-pip \
    python3-pip \
    rpm \
    unzip \
    wget \
    zip \
    zlib1g-dev \
    && rm -rf /var/lib/apt/lists/* \
    && pip install setuptools wheel \
    && pip3 install setuptools wheel

RUN wget -q https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz && \
    tar xf google-cloud-sdk.tar.gz && \
    rm google-cloud-sdk.tar.gz && \
    ./google-cloud-sdk/install.sh
ENV PATH "/google-cloud-sdk/bin:${PATH}"

ENV NODE_VERSION 6.10.0
RUN wget -q https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64.tar.xz && \
    tar xf node-v${NODE_VERSION}-linux-x64.tar.xz --strip-components=1 -C /usr && \
    rm node-v${NODE_VERSION}-linux-x64.tar.xz && \
    npm install -g mocha

RUN INSTALLER="bazel-${BAZEL_VERSION}-installer-linux-x86_64.sh"; \
    DOWNLOAD_URL="https://github.com/bazelbuild/bazel/releases/download/${BAZEL_VERSION}/${INSTALLER}"; \
    wget -q "${DOWNLOAD_URL}" && \
    chmod +x "${INSTALLER}" && "./${INSTALLER}" && rm "${INSTALLER}"

ARG OLD_BAZEL_VERSION
COPY --from=old \
    /usr/local/lib/bazel/bin/bazel-real /usr/local/lib/bazel/bin/bazel-${OLD_BAZEL_VERSION}


WORKDIR /workspace
COPY runner /usr/local/bin/
ENTRYPOINT ["/bin/bash", "/usr/local/bin/runner"]
