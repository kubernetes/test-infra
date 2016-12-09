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

export OWNER="stclair@google.com"
# PROJECT="ci-cadvisor-does-not-use-a-project"
TARGET="google/cadvisor:canary"
FILE="deploy/canary/Dockerfile"
docker build -t "${TARGET}" --no-cache=true --pull=true --file="${FILE}" .
docker inspect "${TARGET}"
if [[ "$(docker version --format='{{.Client.Version}}')" =~ ^1.9 ]]; then
  DOCKER_EMAIL="--email=not@val.id"
else
  DOCKER_EMAIL=
fi
set +o xtrace  # Do not log credentials
echo "Logging in as ${DOCKER_USER}..."
docker login ${DOCKER_EMAIL:-} --username="${DOCKER_USER}" --password="${DOCKER_PASSWORD}"
set -o xtrace
unset DOCKER_USER DOCKER_PASSWORD DOCKER_EMAIL
docker push "${TARGET}"
docker logout
