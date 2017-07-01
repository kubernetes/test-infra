#!/bin/bash
# Copyright 2015 The Kubernetes Authors.
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

# Run e2e tests using environment variables exported in e2e.sh.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

export PS4='+(${BASH_SOURCE}:${LINENO}): ${FUNCNAME[0]:+${FUNCNAME[0]}(): }'

# TODO(fejta): consider moving this elsewhere
echo "--------------------------------------------------------------------------------"
echo "Test Environment:"
printenv | sort
echo "--------------------------------------------------------------------------------"

# TODO(fejta): set KUBETEST_MANUAL_DUMP in kubernetes_e2e.py after new image
if [[ -n "${KUBETEST_MANUAL_DUMP:-}" ]]; then
  e2e_go_args=()
else
  e2e_go_args=( \
    -v \
    --dump="${WORKSPACE}/_artifacts" \
  )
fi

# When run inside Docker, we need to make sure all files are world-readable
# (since they will be owned by root on the host).
for arg in "${@}" "${e2e_go_args[@]}"; do
  if [[ "${arg}" =~ --dump=(.+)$ ]]; then
    dump="${BASH_REMATCH[1]}"
    echo "Will chmod -R o+r ${dump} on EXIT SIGINT SIGTERM"
    trap "chmod -R o+r '${dump}'" EXIT SIGINT SIGTERM
    export E2E_REPORT_DIR="${dump}"  # TODO(fejta): remove after new image
  fi
done

if [[ -n "${KUBE_CONTAINER_RUNTIME:-}" ]]; then
    echo "\$KUBE_CONTAINER_RUNTIME is deprecated"
    GINKGO_TEST_ARGS="${GINKGO_TEST_ARGS:-} --container-runtime=${KUBE_CONTAINER_RUNTIME}"
fi

if [[ -n "${MASTER_OS_DISTRIBUTION:-}" ]]; then
    echo "\$MASTER_OS_DISTRIBUTION is deprecated"
    GINKGO_TEST_ARGS="${GINKGO_TEST_ARGS:-} --master-os-distro=${MASTER_OS_DISTRIBUTION}"
fi

if [[ -n "${NODE_OS_DISTRIBUTION:-}" ]]; then
    echo "\$NODE_OS_DISTRIBUTION is deprecated"
    GINKGO_TEST_ARGS="${GINKGO_TEST_ARGS:-} --node-os-distro=${NODE_OS_DISTRIBUTION}"
fi

if [[ -n "${NUM_NODES:-}" ]]; then
    echo "\$NUM_NODES is deprecated"
    GINKGO_TEST_ARGS="${GINKGO_TEST_ARGS:-} --num-nodes=${NUM_NODES}"
fi

if [[ -n "${E2E_CLEAN_START:-}" ]]; then
    echo "\$E2E_CLEAN_START is deprecated"
    GINKGO_TEST_ARGS="${GINKGO_TEST_ARGS:-} --clean-start=true"
fi

if [[ -n "${E2E_MIN_STARTUP_PODS:-}" ]]; then
    echo "\$E2E_MIN_STARTUP_PODS is deprecated"
    GINKGO_TEST_ARGS="${GINKGO_TEST_ARGS:-} --minStartupPods=${E2E_MIN_STARTUP_PODS}"
fi

if [[ -n "${E2E_REPORT_DIR:-}" ]]; then
    echo "\$E2E_REPORT_DIR is deprecated"
    GINKGO_TEST_ARGS="${GINKGO_TEST_ARGS:-} --report-dir=${E2E_REPORT_DIR}"
fi

if [[ -n "${E2E_REPORT_PREFIX:-}" ]]; then
    echo "\$E2E_REPORT_PREFIX is deprecated"
    GINKGO_TEST_ARGS="${GINKGO_TEST_ARGS:-} --report-prefix=${E2E_REPORT_PREFIX}"
fi

if [[ "${E2E_TEST:-}" == "true" ]]; then
  e2e_go_args+=(--test)
  if [[ "${SKEW_KUBECTL:-}" == 'y' ]]; then
      GINKGO_TEST_ARGS="${GINKGO_TEST_ARGS:-} --kubectl-path=$(pwd)/kubernetes_skew/cluster/kubectl.sh"
  fi
  if [[ -n "${GINKGO_TEST_ARGS:-}" ]]; then
    e2e_go_args+=(--test_args="${GINKGO_TEST_ARGS}")
  fi
fi

kubetest "${e2e_go_args[@]}" "${@}"
