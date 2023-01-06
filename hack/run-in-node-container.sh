#!/usr/bin/env bash
# Copyright 2021 The Kubernetes Authors.
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

# This script runs $@ in a node container

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${REPO_ROOT}"

NODE_IMAGE='node:18-bullseye-slim'

DOCKER=(docker)

if [[ -n "${NO_DOCKER:-}" ]]; then
  DOCKER=(echo docker)
elif ! (command -v docker >/dev/null); then
  echo "WARNING: docker not installed; please install docker or try setting NO_DOCKER=true" >&2
  exit 1
fi

# We are running as the current host user/group so the files produced are
# owned appropriately on the host.
# With rootless mode, this happens without the need for a --user option.
# https://www.redhat.com/sysadmin/user-flag-rootless-containers
# Docker includes the "rootless" keyword in its "system info" output,
# whereas podman includes "rootless: (true|false)".
DOCKER_USER=""
if ! "${DOCKER[@]}" system info | grep -q "rootless\(: true\)\?$"; then
    DOCKER_USER="--user $(id -u):$(id -g)"
fi

# NOTE: yarn tries to read configs under $HOME and fails if it can't,
# we don't need these configs but we need it to not fail.
# We set HOME to somewhere read/write-able by any user, since our uid will not
# exist in /etc/passwd in the node image and yarn will try to read from / and
# fail instead if we don't.
"${DOCKER[@]}" run \
    --rm -i \
    ${DOCKER_USER} \
    -e HOME=/tmp \
    -v "${REPO_ROOT:?}:${REPO_ROOT:?}" -w "${REPO_ROOT}" \
    --security-opt="label=disable" \
    "${NODE_IMAGE}" \
    "$@"
if [[ -n "${NO_DOCKER:-}" ]]; then
  (
    set -o xtrace
    "$@"
  )
fi
