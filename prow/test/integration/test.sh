#!/usr/bin/env bash
# Copyright 2020 The Kubernetes Authors.
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

RUN_INTEGRATION_TEST="false"
for arg in "$@"; do
  if [[ "${arg}" == "--run-integration-test" ]]; then
    RUN_INTEGRATION_TEST="true"
  fi
done
if [[ "$RUN_INTEGRATION_TEST" == "false" ]]; then
  echo "Skip integration test"
  exit 0
fi


CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
readonly DEFAULT_CLUSTER_NAME="kind-prow-integration"
readonly DEFAULT_CONTEXT="kind-${DEFAULT_CLUSTER_NAME}"

GO_DEFAULT_TEST=$(realpath "$1")
shift 1

if [[ -z "${HOME:-}" ]]; then # kubectl looks for HOME which is not set in bazel
  export HOME="$(cd ~ && pwd -P)"
fi

if ! kubectl config get-contexts "${DEFAULT_CONTEXT}"; then
  echo -e "\nError: integration test requires setup, did run 'bazel run //prow/test/integration:setup' yet?\n"
  exit 1
fi

function do_kubectl() {
  kubectl --context=${DEFAULT_CONTEXT} $@
}

function dump_prow_pod_log() {
  if [[ -n "${ARTIFACTS:-}" && -d "${ARTIFACTS}" ]]; then
    local log_dir="${ARTIFACTS}/prow_pod_logs"
    mkdir -p "${log_dir}"
    for svc in sinker; do
      do_kubectl logs svc/$svc >"${log_dir}/${svc}.log"
    done
  fi
}

function main() {
  trap "dump_prow_pod_log" EXIT
  echo "Running integration tests"
  ${GO_DEFAULT_TEST} "$@" || return 1
}

main "$@"
