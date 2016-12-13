#!/usr/bin/env bash

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

readonly KUBEKINS_IMAGE="gcr.io/k8s-testimages/kubekins-e2e"

REPO_ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd)

read upstream patch <<< $( git diff master ${REPO_ROOT}/jenkins/dockerized-e2e-runner.sh | sed -n "/^.KUBEKINS_E2E_IMAGE_TAG=/{s/.*'\(.*\)'/\1/g;p}" ) || true

if [[ -z "$upstream" || -z "$patch" ]]; then
	echo "No image changes detected."
	exit
fi

readonly upstream_image="${KUBEKINS_IMAGE}:${upstream}"
readonly patch_image="${KUBEKINS_IMAGE}:${patch}"

set -x

# grab the images
docker pull ${upstream_image}
docker pull ${patch_image}

# run the image differ
python ${REPO_ROOT}/jenkins/docker_diff.py --deep workspace ${upstream_image} ${patch_image}
