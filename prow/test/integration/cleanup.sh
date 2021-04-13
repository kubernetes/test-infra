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
set -o nounset
set -o pipefail

readonly DEFAULT_CLUSTER_NAME="kind-prow-integration"
readonly DEFAULT_CONTEXT="kind-${DEFAULT_CLUSTER_NAME}"
readonly DEFAULT_REGISTRY_NAME="kind-registry"
readonly PROW_COMPONENTS="sinker crier hook horologium fakeghserver"

function do_kubectl() {
  kubectl --context=${DEFAULT_CONTEXT} $@
}

if [[ -n "${ARTIFACTS:-}" && -d "${ARTIFACTS}" ]]; then
  log_dir="${ARTIFACTS}/prow_pod_logs"
  mkdir -p "${log_dir}"
  for app in ${PROW_COMPONENTS}; do
    do_kubectl logs svc/$app >"${log_dir}/${app}.log"
  done
fi

kind delete cluster --name "${DEFAULT_CLUSTER_NAME}"
container_hash="$(docker ps -q -f name="${DEFAULT_REGISTRY_NAME}" 2>/dev/null || true)"
if [ -z "${container_hash}" ]; then
  echo "Container ${DEFAULT_REGISTRY_NAME} does not exist, skipping cleaning up."
  exit 0
fi
echo "Deleting container ${DEFAULT_REGISTRY_NAME}"
docker container stop "${DEFAULT_REGISTRY_NAME}"
docker container rm "${DEFAULT_REGISTRY_NAME}"
