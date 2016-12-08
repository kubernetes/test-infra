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
git remote remove "${remote}" || true
git remote add "${remote}" "https://github.com/kubernetes/kubernetes.git"
git remote set-url --push "${remote}" no_push
# If .git is cached between runs this data may be stale
git fetch "${remote}"  # fetch branches

export KUBE_FORCE_VERIFY_CHECKS="y"
export KUBE_VERIFY_GIT_BRANCH="release-1.4"
export KUBE_TEST_SCRIPT="./hack/jenkins/verify-dockerized.sh"

### Runner
readonly runner="${testinfra}/jenkins/gotest-dockerized.sh"
export KUBEKINS_TIMEOUT="80m"
timeout -k 15m "${KUBEKINS_TIMEOUT}" "${runner}" && rc=$? || rc=$?

### Reporting
if [[ ${rc} -eq 124 || ${rc} -eq 137 ]]; then
    echo "Build timed out" >&2
elif [[ ${rc} -ne 0 ]]; then
    echo "Build failed" >&2
fi
echo "Exiting with code: ${rc}"
exit ${rc}
