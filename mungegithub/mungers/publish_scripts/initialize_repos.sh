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

ORG="${ORG:-kubernetes}"

mkdir -p "${GOPATH}"/src/k8s.io
cd "${GOPATH}"/src/k8s.io

# Install godep
if [ ! -d $GOPATH/src/github.com/tools/godep ]; then
    go get github.com/tools/godep
fi
pushd /go-workspace/src/github.com/tools/godep
    git fetch
    git checkout tags/v79
    go install ./... 
popd

# Install kube including dependencies
if [ ! -d kubernetes ]; then
    git clone "https://github.com/kubernetes/kubernetes"
    pushd kubernetes
        ./hack/godep-restore.sh
    popd
fi

# Install all staging dirs
for repo in $(cd kubernetes/staging/src/k8s.io; ls -1); do
    if [ -d "${repo}" ]; then
	pushd ${repo}
	    git remote set-url origin "https://github.com/${ORG}/${repo}"
	popd
    else
	git clone "https://github.com/${ORG}/${repo}"
    fi
done
