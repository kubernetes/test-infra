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

retry() {
  end="${1}"
  shift 1
  # Don't quote the regex, will cause wrong output
  [[ "$end" =~ ^[0-9]+$ ]] || ( echo "First param must be integer"; return 1 )
  for attempt in $(seq 1 $end); do
    if eval "$@"; then
      break
    fi
    if [[ "$attempt" != "$end" ]]; then
      echo "****** Command '$@' failed, retrying #${end}... ******"
    fi
  done
}

main() {
  trap "'${bazel}' run //prow/test/integration:cleanup '$@'" EXIT
  "${bazel}" run //prow/test/integration:setup-local-registry "$@" || ( echo "FAILED: set up local registry">&2; return 1 )
  # testimage-push builds images, could fail due to network flakiness
  (retry 3 "'${bazel}' run //prow:testimage-push '$@'") || ( echo "FAILED: pushing images">&2; return 1 )
  "${bazel}" run //prow/test/integration:setup-cluster "$@" || ( echo "FAILED: setup cluster">&2; return 1 )
  "${bazel}" test //prow/test/integration/test:go_default_test --action_env=KUBECONFIG=${HOME}/.kube/config --test_arg=--run-integration-test "$@" || ( echo "FAILED: running tests">&2; return 1 )
}

main "$@"
