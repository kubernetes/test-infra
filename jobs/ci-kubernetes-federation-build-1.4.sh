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

### job-env
export PROJECT="k8s-jkns-e2e-gce-f8n-1-4"
export FEDERATION=true
export FEDERATION_PUSH_REPO_BASE="gcr.io/k8s-jkns-e2e-gce-f8n-1-4"
export KUBE_FASTBUILD=true
export KUBE_GCS_RELEASE_BUCKET=kubernetes-federation-release-1-4
export KUBE_GCS_RELEASE_BUCKET_MIRROR=kubernetes-federation-release-1-4

### Runner
readonly runner="./hack/jenkins/build.sh"
timeout -k 15m 50m "${runner}" && rc=$? || rc=$?

### Reporting
if [[ ${rc} -eq 124 || ${rc} -eq 137 ]]; then
    echo "Build timed out" >&2
elif [[ ${rc} -ne 0 ]]; then
    echo "Build failed" >&2
fi
echo "Exiting with code: ${rc}"
exit ${rc}
