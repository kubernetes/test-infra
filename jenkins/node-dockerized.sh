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

export REPO_DIR=${REPO_DIR:-$(pwd)}
export HOST_ARTIFACTS_DIR=${WORKSPACE}/_artifacts
mkdir -p "${HOST_ARTIFACTS_DIR}"

# Run the kubekins container, mapping in docker (so we can launch containers),
# the repo directory, and the artifacts output directory.
#
# Note: We pass in the absolute path to the repo on the host as an env var incase
# any tests that get run need to launch containers that also map volumes.
# This is required because if you do
#
# $ docker run -v $PATH:/container/path ...
#
# From _inside_ a container that has the host's docker mapped in, the $PATH
# provided must be resolvable on the *HOST*, not the container.

# default to go version 1.6 image tag
NODEIMAGE_TAG="1.6-latest"

if [[ "${NODE_IMG_VERSION}" == *"1.2" ]] || \
   [[ "${NODE_IMG_VERSION}" == *"1.3" ]] || \
   [[ "${NODE_IMG_VERSION}" == *"1.4" ]]; then
  NODEIMAGE_TAG="1.4-latest"
elif [[ "${NODE_IMG_VERSION}" == *"1.5" ]]; then
  NODEIMAGE_TAG="1.5-latest"
fi

# run node test as jenkins
GCE_USER="jenkins"

# force pull the image since we are using latest tag
docker pull "gcr.io/k8s-testimages/kubekins-node:${NODEIMAGE_TAG}"

docker run --rm=true \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "${REPO_DIR}":/go/src/k8s.io/kubernetes \
  -v "${WORKSPACE}/_artifacts":/workspace/_artifacts \
  -v /etc/localtime:/etc/localtime:ro \
  ${JENKINS_GCE_SSH_PRIVATE_KEY_FILE:+-v "${JENKINS_GCE_SSH_PRIVATE_KEY_FILE}:/root/.ssh/google_compute_engine:ro"} \
  ${JENKINS_GCE_SSH_PUBLIC_KEY_FILE:+-v "${JENKINS_GCE_SSH_PUBLIC_KEY_FILE}:/root/.ssh/google_compute_engine.pub:ro"} \
  ${GOOGLE_APPLICATION_CREDENTIALS:+-v "${GOOGLE_APPLICATION_CREDENTIALS}:/service-account.json:ro"} \
  -e "GCE_USER=${GCE_USER}" \
  -e "REPO_DIR=${REPO_DIR}" \
  -e "HOST_ARTIFACTS_DIR=${HOST_ARTIFACTS_DIR}" \
  -e "NODE_TEST_SCRIPT=${NODE_TEST_SCRIPT}" \
  -e "NODE_TEST_PROPERTIES=${NODE_TEST_PROPERTIES}" \
  ${GOOGLE_APPLICATION_CREDENTIALS:+-e "GOOGLE_APPLICATION_CREDENTIALS=/service-account.json"} \
  -i "gcr.io/k8s-testimages/kubekins-node:${NODEIMAGE_TAG}"

