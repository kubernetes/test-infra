#!/bin/bash
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

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

# Install dependencies required by the kubernetes-anywhere e2e runner: jsonnet, terraform
#
# The below is adapted from
#   https://github.com/kubernetes/kubernetes-anywhere/blob/master/util/docker-build.sh
# but modified to work with our base image.

apt-get update
apt-get install -y unzip

# Install jsonnet
cd /tmp
git clone https://github.com/google/jsonnet.git
(cd jsonnet
make jsonnet
cp jsonnet /bin)
rm -rf /tmp/jsonnet

# Install terraform
export TERRAFORM_VERSION=0.7.2

mkdir -p /tmp/terraform/
(cd /tmp/terraform
wget https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip
unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip -d /bin)
rm -rf /tmp/terraform
