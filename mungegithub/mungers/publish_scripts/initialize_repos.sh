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
# This script is expected to be run by the Dockerfile or an init container.

set -o errexit
set -o nounset
set -o pipefail

ORG="${ORG:-kubernetes}"

mkdir -p "${GOPATH}"/src/k8s.io
cd "${GOPATH}"/src/k8s.io

# Install go tools
go get github.com/tools/godep
pushd ${GOPATH}/src/github.com/tools/godep
    git checkout tags/v79
    go install ./...
popd

go get github.com/golang/dep
pushd ${GOPATH}/src/github.com/golang/dep
    git checkout 7c44971bbb9f0ed87db40b601f2d9fe4dffb750d
    go install ./cmd/dep
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

    pushd ${repo}
        git config user.name "${GIT_COMMITTER_NAME}"
        git config user.email "${GIT_COMMITTER_EMAIL}"
    popd
done
