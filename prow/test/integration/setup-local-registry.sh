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

set -o errexit

readonly DEFAULT_CONTEXT="kind-${DEFAULT_CLUSTER_NAME}"
readonly DEFAULT_REGISTRY_NAME="kind-registry"
readonly DEFAULT_REGISTRY_PORT="5000"

# create registry container unless it already exists
running="$(docker inspect -f '{{.State.Running}}' "${DEFAULT_REGISTRY_NAME}" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  echo "Creating docker container for hosting local registry localhost:${DEFAULT_REGISTRY_PORT}"
  docker run \
    -d --restart=always -p "127.0.0.1:${DEFAULT_REGISTRY_PORT}:${DEFAULT_REGISTRY_PORT}" --name "${DEFAULT_REGISTRY_NAME}" \
    registry:2
else
  echo "Local registry localhost:${DEFAULT_REGISTRY_PORT} already exist and running."
fi
