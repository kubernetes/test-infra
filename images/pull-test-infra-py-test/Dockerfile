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
    pylint \
    python-nose && \
    apt-get clean

RUN wget https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-136.0.0-linux-x86_64.tar.gz && \
    tar xf google-cloud-sdk-136.0.0-linux-x86_64.tar.gz && \
    rm google-cloud-sdk-136.0.0-linux-x86_64.tar.gz && \
    ./google-cloud-sdk/install.sh
ENV PATH="${PATH}:/google-cloud-sdk/bin"

RUN mkdir -p /workspace
WORKDIR /workspace
ADD runner /
ENTRYPOINT ["/bin/bash", "/runner"]
