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

CURRENT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd -P )"

function dump_prow_pod_log() {
  local log_dir="${ARTIFACTS}/prow_pod_logs"
  if [[ -n "${ARTIFACTS:-}" && -d "${ARTIFACTS}" ]]; then
    mkdir -p "${log_dir}"
    for svc in sinker; do
      kubectl logs svc/$svc >"${log_dir}/${svc}.log"
    done
  fi
}

# TODO(chaodaiG): remove this once kind is installed in test image
if ! which kind >/dev/null 2>&1; then
  echo "Install KIND for prow"
  curl -Lo /usr/bin/kind https://kind.sigs.k8s.io/dl/v0.9.0/kind-linux-amd64
  chmod +x /usr/bin/kind
fi

# TODO(chaodaiG): remove this once bazel is installed in test image
if ! which bazel >/dev/null 2>&1 || [[ "$(bazel --version)" != "bazel 3.0.0" ]]; then
  echo "Install bazel for prow"
  mkdir -p "/usr/local/lib/bazel/bin"
  pushd "/usr/local/lib/bazel/bin" >/dev/null
  curl -LO https://releases.bazel.build/3.0.0/release/bazel-3.0.0-linux-x86_64
  chmod +x bazel-3.0.0-linux-x86_64
  popd
fi

trap 'dump_prow_pod_log' EXIT

"${CURRENT_DIR}/setup-cluster.sh" "$@"
"${CURRENT_DIR}/setup-prow.sh" "$@"

bazel test //prow/test/integration/test:go_default_test --action_env=KUBECONFIG=${HOME}/.kube/config --test_tag_filters=e2e "$@"
