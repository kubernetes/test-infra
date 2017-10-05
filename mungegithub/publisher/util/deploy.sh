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

set -o errexit
set -o nounset
set -o pipefail

if [ ! $# -eq 3 ]; then
    echo "usage: $0 github_token kubectl_context image_repo"
    exit 1
fi

# Path to github token the robot uses to push changes to repositories.
# Used by the munger's Makefile
TOKEN="${1}"
# The kubectl context determines where the robot is deployed to.
CONTEXT="${2}"
# The repo to push the docker image of the robot.
# Used by the munger's Makefile
# Use "gcr.io/google_containers" for real deploy.
REPO="${3}"

MUNGERS_ROOT=$(dirname "${BASH_SOURCE}")/../..
cd "${MUNGERS_ROOT}"

echo "${TOKEN}" > token

KUBECTL="kubectl --context ${CONTEXT}"

${KUBECTL} delete deployment kubernetes-publisher || true
${KUBECTL} delete secret kubernetes-github-token || true
${KUBECTL} delete configmap kubernetes-publisher-config || true

make secret volume deployment APP=publisher TARGET=kubernetes REPO="${REPO}"

${KUBECTL} apply -f ./publisher/storage-class.yaml || true
${KUBECTL} apply -f ./publisher/local.pvc.yaml || true
${KUBECTL} apply -f ./publisher/local.pv.yaml || true

make push_secret push_config deploy READONLY=false APP=publisher TARGET=kubernetes REPO="${REPO}"
