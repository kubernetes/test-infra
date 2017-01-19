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
case "${PULL_BASE_REF:-}" in
release-1.0|release-1.1|release-1.2|release-1.3)
  echo "PR Kubemark e2e GCE job disabled for legacy branches."
  exit 0
  ;;
esac

export KUBE_GCS_RELEASE_BUCKET=kubernetes-release-pull
export KUBE_GCS_RELEASE_SUFFIX="/${JOB_NAME}"
export KUBE_GCS_UPDATE_LATEST=n
export JENKINS_USE_LOCAL_BINARIES=y
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
export KUBEMARK_TESTS="\[Feature:Empty\]"
export KUBEMARK_TEST_ARGS="--gather-resource-usage=true --garbage-collector-enabled=true"
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

# Force to use GCI.
export KUBE_NODE_OS_DISTRIBUTION="gci"

# Skip gcloud update checking
export CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true

# Get golang into our PATH so we can run e2e.go
export PATH=${PATH}:/usr/local/go/bin

readonly runner="${testinfra}/jenkins/dockerized-e2e-runner.sh"
export KUBEKINS_TIMEOUT="45m"
"${runner}"
