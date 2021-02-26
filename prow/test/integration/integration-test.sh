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
  for attempt in $(seq 1 3); do
    if "$@"; then
      break
    fi
    if [[ "$attempt" != "3" ]]; then
      echo "****** Command '$@' failed, retrying #${attempt}... ******"
    fi
  done
}

main() {
  trap "'${bazel}' run //prow/test/integration:cleanup '$@'" EXIT
  "${bazel}" run //prow/test/integration:setup-local-registry "$@" || ( echo "FAILED: set up local registry">&2; return 1 )
  # testimage-push builds images, could fail due to network flakiness
  (retry "${bazel}" run //prow:testimage-push "$@") || ( echo "FAILED: pushing images">&2; return 1 )
  "${bazel}" run //prow/test/integration:setup-cluster "$@" || ( echo "FAILED: setup cluster">&2; return 1 )
  "${bazel}" test //prow/test/integration/test:go_default_test --action_env=KUBECONFIG=${HOME}/.kube/config --test_arg=--run-integration-test "$@" || ( echo "FAILED: running tests">&2; return 1 )
}

time bazel test --config=ci --nobuild_tests_only //...

time main "$@"
