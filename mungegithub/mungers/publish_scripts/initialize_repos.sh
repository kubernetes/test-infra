#!/bin/bash

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

# This script warms up the ${GOPATH}, so the first run of publisher will not take too long.
# This script is expected to be run by the Dockerfile.

set -o errexit
set -o nounset
set -o pipefail

ORG="kubernetes"

mkdir -p "${GOPATH}"/src/k8s.io
cd "${GOPATH}"/src/k8s.io

# to restore the dependencies
git clone "https://github.com/kubernetes/kubernetes"
pushd kubernetes
    ./hack/godep-restore.sh
popd

for repo in "apimachinery" "client-go" "apiserver" "kube-aggregator" "sample-apiserver" "apiextensions-apiserver"
do
    git clone "https://github.com/${ORG}/${repo}"
done
