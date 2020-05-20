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

FROM ubuntu:16.04

RUN apt-get update && apt-get install -y \
    git \
    wget \
    unzip \
    python \
    python-pip \
    mocha && \
    rm -rf /var/lib/apt/lists/*

ENV GCLOUD_VERSION 138.0.0
RUN wget https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-$GCLOUD_VERSION-linux-x86_64.tar.gz && \
    tar xf google-cloud-sdk-$GCLOUD_VERSION-linux-x86_64.tar.gz && \
    rm google-cloud-sdk-$GCLOUD_VERSION-linux-x86_64.tar.gz && \
    ./google-cloud-sdk/install.sh
ENV PATH "/google-cloud-sdk/bin:${PATH}"

# Based on https://github.com/travis-ci/travis-ci/issues/738#issuecomment-11179888
ENV GAE_ZIP=google_appengine_1.9.40.zip GAE_ROOT=/google_appengine
RUN wget -nv https://storage.googleapis.com/appengine-sdks/featured/${GAE_ZIP} && \
    unzip -q ${GAE_ZIP} -d /

WORKDIR /workspace
ADD runner /
ENTRYPOINT ["/bin/bash", "/runner"]
