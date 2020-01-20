#!/bin/bash
#
# Copyright 2019 The Kubernetes Authors.
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

set -eux

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get -qqy --no-install-recommends install \
  apt-transport-https \
  build-essential \
  ca-certificates \
  curl \
  lsb-release \
  software-properties-common \
  ssh \
  unzip \
  wget \
  zip \
  jq

# install golang
GO_VERSION='1.13'
GOARCH=$(uname -m | sed 's/x86_64/amd64/g')
GO_BASE_URL="https://storage.googleapis.com/golang"
GO_ARCHIVE="go${GO_VERSION}.linux-${GOARCH}.tar.gz"
GO_URL="${GO_BASE_URL}/${GO_ARCHIVE}"

export GOPATH=/home/go
mkdir -p ${GOPATH}/bin
curl -L "${GO_URL}" | tar -C /usr/local -xzf -
export PATH=${GOPATH}/bin:/usr/local/go/bin:${PATH}

rm -rf "${GO_ARCHIVE}"

# install docker
DOCKER_ARCH=$(dpkg --print-architecture)
DOCKER_VERSION=18.06.1
VERSION_SUFFIX="~ce~3-0~ubuntu"

curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -
add-apt-repository "deb [arch=${DOCKER_ARCH}] https://download.docker.com/linux/ubuntu \
 $(lsb_release -cs) stable"
apt-get update
apt-get -qqy install docker-ce="${DOCKER_VERSION}${VERSION_SUFFIX}"

# install gcloud
export CLOUDSDK_PYTHON_SITEPACKAGES=1
export CLOUDSDK_CORE_DISABLE_PROMPTS=1
export CLOUDSDK_INSTALL_DIR=/usr/lib/
curl https://sdk.cloud.google.com | bash

export PATH="${PATH}:/usr/lib/google-cloud-sdk/bin"

sed -i -e 's/true/false/' /usr/lib/google-cloud-sdk/lib/googlecloudsdk/core/config.json
gcloud -q components update

# install kubectl
curl -LO https://storage.googleapis.com/kubernetes-release/release/"$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)"/bin/linux/${GOARCH}/kubectl
chmod +x ./kubectl
mv ./kubectl /usr/local/bin/kubectl

apt-get clean
rm -rf /var/lib/apt/lists/*
