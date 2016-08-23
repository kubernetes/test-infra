#!/bin/bash

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

# usage: deploy.sh [container_registry]

set -o errexit
set -o nounset

IMAGE="${1:-gcr.io/google_containers}/fetcher-$(date +%Y-%M-%d)-$(git rev-parse --verify --short HEAD)"

set -o xtrace

cd "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CGO_ENABLED=0 go build .
docker build -t "${IMAGE}" .
gcloud docker push "${IMAGE}"
cat deployment.yaml | env - IMAGE="${IMAGE}" envsubst | kubectl apply -f -
