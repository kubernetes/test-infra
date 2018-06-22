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

#
# Use like: ./planter/planter.sh bazel build //cmd/...
#
# - $TAG can be overridden to choose a bazel version eg `TAG=0.8.0 planter.sh ...`
# - $DOCKER_EXTRA can be set to supply extra args in the docker call
# - $DRY_RUN will trigger echoing the docker call instead of running it
# - planter will always `docker pull $IMAGE` before running it, see below
#
# To build kubernetes, first you checkout github.com/kubernetes/kubernetes
# to $GOPATH/src/k8s.io/kubernetes and github.com/kubernetes/test-infra
# to $GOPATH/src/k8s.io/test-infra.
#
# Then you can build with:
# $ cd $GOPATH/src/k8s.io/kubernetes`
# $ $GOPATH/src/k8s.io/test-infra/planter/planter.sh make bazel-build`
#

set -o errexit
set -o nounset
IMAGE_NAME="gcr.io/k8s-testimages/planter"
TAG="${TAG:-0.14.0}"
IMAGE="${IMAGE_NAME}:${TAG}"
# We want to mount our bazel workspace and the bazel cache
# - WORKSPACE is assumed to be in your current git repo, or alternatively $PWD
REPO=$(git rev-parse --show-toplevel 2>/dev/null || true)
REPO=${REPO:-${PWD}}

# Ensure the bazel cache directory exists, as otherwise docker will create it,
# owned by root.
BAZEL_CACHE="${HOME}/.cache/bazel"
[[ -d "${BAZEL_CACHE}" ]] || mkdir -p "${BAZEL_CACHE}"

# Instead of mounting the full user ${HOME} which previously causes many issues like #5607 & #5736
# We mount only the following folders:
# - ${HOME}/.cache/bazel to share bazel cache across builds
# - /tmp also needs to be a suitable tmpfs mounted with exec so that bazel
# can use it when executing various things
# We also use the delegated option on the mounts to improve performance on macOS.
# https://docs.docker.com/docker-for-mac/osxfs-caching/
VOLUMES="-v ${REPO}:${REPO}:delegated -v ${BAZEL_CACHE}:${BAZEL_CACHE}:delegated --tmpfs /tmp:exec,mode=777"

# We want to run as the host user so they own the build outputs etc.
# Part of this is handled in planter/entrypoint.sh
GID="$(id -g ${USER})"
ENV="-e USER=${USER} -e GID=${GID} -e UID=${UID} -e HOME=${HOME}"
# construct the final docker command, with SELinux disabled for this container
CMD="docker pull ${IMAGE} && docker run --security-opt label:disable --rm ${VOLUMES} --user ${UID}:${GID} -w ${PWD} ${ENV} ${DOCKER_EXTRA:-} ${IMAGE} ${@}"
if [ -n "${DRY_RUN+set}" ]; then
    echo "${CMD}"
else
    eval ${CMD}
fi
