#!/bin/sh
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

# NOTE: use like: ./planter.sh bazel build //cmd/...
# NOTE: build kubernetes with: <path to test-infra>/planter.sh make bazel-build

set -o errexit
set -o nounset
IMAGE_NAME="gcr.io/k8s-testimages/planter"
TAG="${TAG:-0.7.0-1}"
IMAGE="${IMAGE_NAME}:${TAG}"
# run our docker image as the host user with bazel cache and current repo dir
REPO=$(git rev-parse --show-toplevel 2>/dev/null || true)
REPO=${REPO:-${PWD}}
VOLUMES="-v ${REPO}:${REPO} -v ${HOME}:${HOME} --tmpfs /tmp:exec,mode=777"
GID="$(id -g ${USER})"
ENV="-e USER=${USER} -e GID=${GID} -e UID=${UID} -e HOME=${HOME}"
# the final command to run
CMD="docker run --rm ${VOLUMES} --user ${UID} -w ${PWD} ${ENV} ${DOCKER_EXTRA:-} ${IMAGE} ${@}"
if [ -n "${DRY_RUN+set}" ]; then
    echo "${CMD}"
else
    ${CMD}
fi
