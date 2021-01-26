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

# https://hub.docker.com/_/ubuntu?tab=tags
# https://wiki.ubuntu.com/Releases
FROM ubuntu:bionic-20200526


RUN apt update && apt install -y \
      git \
      mocha \
      python \
      python-pip \
      unzip \
      wget \
    && rm -rf /var/lib/apt/lists/*

ENV GAE_ZIP=google_appengine_1.9.40.zip GAE_ROOT=/google_appengine
RUN touch /etc/apt/sources.list.d/google-cloud-sdk.list \
    && echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] http://packages.cloud.google.com/apt cloud-sdk main" \
    >> /etc/apt/sources.list.d/google-cloud-sdk.list \
    && wget -O - https://packages.cloud.google.com/apt/doc/apt-key.gpg \
    | apt-key --keyring /usr/share/keyrings/cloud.google.gpg add - \
    && apt update && apt install -y \
      google-cloud-sdk \
    && rm -rf /var/lib/apt/lists/* \
    && wget -nv https://storage.googleapis.com/appengine-sdks/featured/${GAE_ZIP} \
    && unzip -q ${GAE_ZIP} -d /

WORKDIR /workspace
