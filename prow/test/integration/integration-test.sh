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

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd "${REPO_ROOT}"
source hack/build/setup-go.sh

# build gotestsum
cd 'hack/tools'
go build -o "${REPO_ROOT}/_bin/gotestsum" gotest.tools/gotestsum
cd "${REPO_ROOT}"

function retry() {
  for attempt in $(seq 1 3); do
    if "$@"; then
      break
    fi
    if [[ "$attempt" != "3" ]]; then
      echo "****** Command '$@' failed, retrying #${attempt}... ******"
    fi
  done
}

function setup() {
  ./prow/test/integration/setup-local-registry.sh

  go run \
    ./hack/prowimagebuilder \
    --ko-docker-repo="localhost:5001" \
    --prow-images-file="prow/test/integration/prow/.prow-images.yaml" \
    --push
  
  ./prow/test/integration/setup-cluster.sh
}

function run() {
  JUNIT_RESULT_DIR="${REPO_ROOT}/_output"
  # if we are in CI, copy to the artifact upload location
  if [[ -n "${ARTIFACTS:-}" ]]; then
    JUNIT_RESULT_DIR="${ARTIFACTS}"
  fi

  # run integration tests with junit output
  (
    set -x;
    mkdir -p "${JUNIT_RESULT_DIR}"
    "${REPO_ROOT}/_bin/gotestsum" --junitfile="${JUNIT_RESULT_DIR}/junit-integration.xml" \
      -- "./prow/test/integration/test" --run-integration-test
  )
}

function teardown() {
  ./prow/test/integration/cleanup.sh
}

function main() {
  if [[ "${SKIP_TEARDOWN:-}" != "true" ]]; then
    trap "teardown '$@'" EXIT
  fi
  if [[ "${SKIP_SETUP:-}" != "true" ]]; then
    setup "$@"
  fi

  run "$@"
  exit 0
}

main
