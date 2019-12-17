#!/bin/bash
#
# Copyright 2019 The Kubernetes Authors.
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

DOCKER_REGISTRY="quay.io"
DOCKER_USERNAME="multicloudlab"

# support other container tools, e.g. podman
CONTAINER_CLI=${CONTAINER_CLI:-docker}
HUB="${DOCKER_REGISTRY}/${DOCKER_USERNAME}"
IMAGE="build-tools"
VERSION=$(date +v%Y%m%d)-$(git describe --tags --always --dirty)

${CONTAINER_CLI} build -t "${HUB}/${IMAGE}:${VERSION}" .
${CONTAINER_CLI} push "${HUB}/${IMAGE}:${VERSION}"
