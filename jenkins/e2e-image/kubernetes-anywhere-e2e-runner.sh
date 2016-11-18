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

# Download the latest version of kubernetes-anywhere, then run the e2e tests using e2e-runner.sh.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

export KUBERNETES_ANYWHERE="${WORKSPACE}/kubernetes-anywhere"

git clone https://github.com/kubernetes/kubernetes-anywhere.git "${KUBERNETES_ANYWHERE}"

export E2E_OPT="--deployment kubernetes-anywhere --kubernetes-anywhere-path \"${KUBERNETES_ANYWHERE}\" ${E2E_OPT}"

$(dirname "${BASH_SOURCE}")/e2e-runner.sh
