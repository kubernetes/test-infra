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
# Environment variable options:
# - $TAG can be overridden to choose a bazel version eg `TAG=0.8.0 planter.sh ...`
# - Alternatively $IMAGE or $IMAGE_NAME can be overridden to set the exact image
# - $NO_PULL will disable pulling the image before running if set
# - $DOCKER_EXTRA can be set to supply extra args in the docker call
# - $DRY_RUN will trigger echoing the docker call instead of running it
#
# To build kubernetes, first you checkout github.com/kubernetes/kubernetes
# to $GOPATH/src/k8s.io/kubernetes and github.com/kubernetes/test-infra
# to $GOPATH/src/k8s.io/test-infra.
#
# Then you can build with:
# $ cd $GOPATH/src/k8s.io/kubernetes
# $ $GOPATH/src/k8s.io/test-infra/planter/planter.sh make bazel-release
# 
# or build a specific binary like:
# $ cd $GOPATH/src/k8s.io/kubernetes
# $ $GOPATH/src/k8s.io/test-infra/planter/planter.sh bazel build //cmd/kubectl
#

set -o errexit
set -o nounset

# these can be overridden but otherwise default to the current stable image
# used to build kubernetes from the master branch
IMAGE_NAME="${IMAGE_NAME:-gcr.io/k8s-testimages/planter}"
TAG="${TAG:-0.18.1}"
IMAGE=${IMAGE:-${IMAGE_NAME}:${TAG}}

# We want to mount our bazel workspace and the bazel cache
# - WORKSPACE is assumed to be in your current git repo, or alternatively $PWD
REPO=$(git rev-parse --show-toplevel 2>/dev/null || true)
REPO=${REPO:-${PWD}}

# Ensure the bazel cache directory exists, as otherwise docker will create it,
# possibly with the wrong owner.
BAZEL_CACHE="${HOME}/.cache/bazel"
if [ ! -d "${BAZEL_CACHE}" ] ; then
    mkdir -p "${BAZEL_CACHE}"
fi

# Construct the set of docker run options (RUN_OPTS)
# - Don't keep the container after it exits
RUN_OPTS="--rm"
# - run interactively, which gives a better user experience
RUN_OPTS="${RUN_OPTS} -it"

# we want a fixed hostname because this leaks into the env when building
RUN_OPTS="${RUN_OPTS} --hostname=planter"

# - NOTE: SELinux disabled for this container to prevent relabeling $HOME (!)
RUN_OPTS="${RUN_OPTS} --security-opt label:disable"

# - supply the host user's `id` info so we can run as the host user and create
#   a matching environment for the purposes of building, see the Dockerfile and
#   entrypoint.sh for more details.
_UID="$(id -u "${USER}")"
_GID="$(id -g "${USER}")"
RUN_OPTS="${RUN_OPTS} --user ${_UID}:${_GID} -e GID=${_GID} -e UID=${_UID}"
# macOS has the convenient `id -F` and no getent, unix has getent
if command -v getent >/dev/null 2>&1; then
    FULL_NAME="$(getent passwd "${USER}" | cut -d : -f 5)"
else
    FULL_NAME="$(id -F "${USER}")"
fi
RUN_OPTS="${RUN_OPTS} -e USER=${USER} -e FULL_NAME='${FULL_NAME}'"
RUN_OPTS="${RUN_OPTS} -e HOME=${HOME}"

# - We mount the following folders:
# + `/tmp` which needs to be a suitable tmpfs mounted with exec so that bazel
#   can use it when executing various things
RUN_OPTS="${RUN_OPTS} --tmpfs /tmp:exec,mode=777"

# + `${HOME}/.cache/bazel` to share bazel cache across builds
RUN_OPTS="${RUN_OPTS} -v ${BAZEL_CACHE}:${BAZEL_CACHE}:delegated"

# + `${REPO}` so we can build the code in REPO
RUN_OPTS="${RUN_OPTS} -v ${REPO}:${REPO}:delegated"

# NOTE: We use the delegated option on both persistent mounts to improve
# performance on macOS. https://docs.docker.com/docker-for-mac/osxfs-caching/

# - we set the working directory to $PWD, which already must have been in $REPO
#   This needs to be set at runtime because $REPO isn't known when we build the
#   planter image.
#   $PWD specifically can also make commands other than bazel more consistent
RUN_OPTS="${RUN_OPTS} -w ${PWD}"

# Preserve GOPATH if any
if [ -n "${GOPATH:-}" ]; then
    RUN_OPTS="${RUN_OPTS} -e GOPATH=${GOPATH}"
fi

# - pass through any extra user-supplied options
if [ -n "${DOCKER_EXTRA:-}" ]; then
    RUN_OPTS="${RUN_OPTS} ${DOCKER_EXTRA}"
fi

# this is the command we will actually run
CMD="docker run ${RUN_OPTS} ${IMAGE} ${1+"$@"}"
# if not NO_PULL then including pulling the image before running
if [ -z "${NO_PULL+set}" ]; then
    CMD="docker pull ${IMAGE} && ${CMD}"
fi

# run the command or echo it if dry run
if [ -z "${DRY_RUN+set}" ]; then
    eval "${CMD}"
else
    echo "${CMD}"
fi
