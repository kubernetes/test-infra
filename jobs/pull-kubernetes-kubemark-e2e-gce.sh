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

# TODO(fejta): remove this
if [[ "${ghprbTargetBranch:-}" == "release-1.0" \
      || "${ghprbTargetBranch:-}" == "release-1.1" \
      || "${ghprbTargetBranch:-}" == "release-1.2" ]]; then
  echo "PR GCE job disabled for legacy branches."
  exit
fi

export KUBE_SKIP_PUSH_GCS=y
export KUBE_RUN_FROM_OUTPUT=y
export KUBE_FASTBUILD=true
./hack/jenkins/build.sh

# Nothing should want Jenkins $HOME
export HOME=${WORKSPACE}
export KUBERNETES_PROVIDER="gce"

# Having full "kubemark" in name will result in exceeding allowed length
# of firewall-rule name.
export E2E_NAME="k6k-e2e-${NODE_NAME}-${EXECUTOR_NUMBER}"
export PROJECT="k8s-jkns-pr-kubemark"
export E2E_UP="true"
export E2E_TEST="false"
export E2E_DOWN="true"
export USE_KUBEMARK="true"
export KUBEMARK_TESTS="starting\s30\spods\sper\snode"
export FAIL_ON_GCP_RESOURCE_LEAK="false"

# Override defaults to be independent from GCE defaults and set kubemark parameters
export NUM_NODES="1"
export MASTER_SIZE="n1-standard-1"
export NODE_SIZE="n1-standard-2"
export KUBE_GCE_ZONE="us-central1-f"
export KUBEMARK_MASTER_SIZE="n1-standard-1"
export KUBEMARK_NUM_NODES="5"

# The kubemark scripts build a Docker image
export JENKINS_ENABLE_DOCKER_IN_DOCKER="y"

# GCE variables
export INSTANCE_PREFIX=${E2E_NAME}
export KUBE_GCE_NETWORK=${E2E_NAME}
export KUBE_GCE_INSTANCE_PREFIX=${E2E_NAME}

# Force to use container-vm.
export KUBE_NODE_OS_DISTRIBUTION="debian"

# Skip gcloud update checking
export CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true

# Get golang into our PATH so we can run e2e.go
export PATH=${PATH}:/usr/local/go/bin

readonly runner="${testinfra}/jenkins/dockerized-e2e-runner.sh"
timeout -k 15m 45m {runner} && rc=$? || rc=$?
if [[ ${rc} -ne 0 ]]; then
  if [[ -x cluster/log-dump.sh && -d _artifacts ]]; then
    echo "Dumping logs for any remaining nodes"
    ./cluster/log-dump.sh _artifacts
  fi
fi
if [[ ${rc} -eq 124 || ${rc} -eq 137 ]]; then
  echo "Build timed out" >&2
elif [[ ${rc} -ne 0 ]]; then
  echo "Build failed" >&2
fi
echo "Exiting with code: ${rc}"
exit ${rc}
