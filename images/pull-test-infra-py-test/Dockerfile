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

FROM ubuntu:16.04
MAINTAINER spxtr@google.com

RUN apt-get update && apt-get install -y \
    build-essential \
    git \
    wget \
    python \
    python-pip \
    unzip && \
    apt-get clean

ENV GCLOUD_TAR="google-cloud-sdk-136.0.0-linux-x86_64.tar.gz" \
    GAE_ZIP="google_appengine_1.9.40.zip" \
    GAE_ROOT="/workspace/google_appengine" \
    PATH="/google-cloud-sdk/bin:${PATH}"

RUN mkdir -p /workspace

RUN wget "https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/${GCLOUD_TAR}" && \
    tar xf "${GCLOUD_TAR}" && \
    rm "${GCLOUD_TAR}" && \
    ./google-cloud-sdk/install.sh

RUN wget -nv "https://storage.googleapis.com/appengine-sdks/featured/${GAE_ZIP}" && \
    unzip -q "${GAE_ZIP}" -d /workspace

RUN pip install webtest nosegae pylint==1.6.4 coverage requests jinja2 pyyaml

# We need these set for a few test cases.
RUN git config --global user.email "you@example.com" && \
    git config --global user.name "Your Name"

WORKDIR /workspace
ADD runner /
ENTRYPOINT ["/bin/bash", "/runner"]
