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

bazel=$(command -v bazelisk || command -v bazel)

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
  "${bazel}" run //prow/test/integration:setup-local-registry "$@" || ( echo "FAILED: set up local registry">&2; return 1 )

  if [[ "${PUSH_IMAEG_WITH_KO:-}" == "true" ]]; then
    PUSH=true KO_DOCKER_REPO="localhost:5001" ./prow/prow-images.sh
  else
    # testimage-push builds images, could fail due to network flakiness
    (retry "${bazel}" run //prow:testimage-push "$@") || ( echo "FAILED: pushing images">&2; return 1 )
  fi
  "${bazel}" run //prow/test/integration:setup-cluster "$@" || ( echo "FAILED: setup cluster">&2; return 1 )
}

function run() {
  "${bazel}" test //prow/test/integration/test:go_default_test --action_env=KUBECONFIG="${HOME}/.kube/config" --test_arg=--run-integration-test "$@" || \
    ( echo "FAILED: running tests">&2; return 1 )
}

function teardown() {
  "${bazel}" run //prow/test/integration:cleanup "$@"
}

function main() {
  # Remove retries after
  # https://github.com/bazelbuild/bazel/issues/12599
  if [ -n "${BAZEL_FETCH_PLEASE:-}" ]; then
    (retry "${bazel}" fetch //prow/test/integration:cleanup //prow/test/integration:setup-local-registry //prow:testimage-push //prow/test/integration:setup-cluster //prow/test/integration/test:go_default_test) || ( echo "FAILED: bazel fetch">&2; return 1 )
  fi

  trap "teardown '$@'" EXIT
  setup "$@"

  run "$@"
  exit 0
}

# If no parameters were given or the first parameter is a flag
if [ $# -eq 0 ] || [[ "${1:-main}" =~ ^- ]]; then
  main "$@"
fi

declare -F "${1}" || (echo "Function \"${1}\" is unavailable. Please use one of" && compgen -A function && exit 1)

"$@"
