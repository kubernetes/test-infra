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

readonly testinfra="$(dirname "${0}")/.."
readonly remote="bootstrap-upstream"

rm -rf .gsutil  # This causes verify flags to fail...
git remote remove "${remote}" 2>/dev/null || true
git remote add "${remote}" "https://github.com/kubernetes/kubernetes.git"
git remote set-url --push "${remote}" no_push
# If .git is cached between runs this data may be stale
git fetch "${remote}"  # fetch branches
export KUBE_VERIFY_GIT_BRANCH="${PULL_BASE_REF}"
export KUBE_TEST_SCRIPT="./hack/jenkins/verify-dockerized.sh"
${testinfra}/jenkins/gotest-dockerized.sh
