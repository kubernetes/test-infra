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

FROM ubuntu:22.04

RUN apt-get update && apt-get install -y \
    git \
    jq \
    wget \
    python3 \
    python3-pip && \
    rm -rf /var/lib/apt/lists/*

# TODO: these are quite old ...
ARG CLOUD_SDK_VERSION=390.0.0
ARG BIGQUERY_LIBRARY_VERSION=0.26.0
ARG RUAMEL_VERSION=0.16

RUN pip3 install --no-cache-dir google-cloud-bigquery==${BIGQUERY_LIBRARY_VERSION} ruamel.yaml==${RUAMEL_VERSION}

ENV PATH=/google-cloud-sdk/bin:${PATH} \
    CLOUDSDK_CORE_DISABLE_PROMPTS=1

RUN pip3 install -U crcmod==1.7
RUN wget https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-${CLOUD_SDK_VERSION}-linux-x86_64.tar.gz
RUN tar xzf google-cloud-sdk-${CLOUD_SDK_VERSION}-linux-x86_64.tar.gz -C /
RUN rm google-cloud-sdk-${CLOUD_SDK_VERSION}-linux-x86_64.tar.gz
RUN gcloud config set core/disable_usage_reporting true && \
    gcloud config set component_manager/disable_update_check true && \
    gcloud config set metrics/environment github_docker_image && \
    gcloud --version

WORKDIR /workspace
ADD runner /
ENTRYPOINT ["/bin/bash", "/runner"]
