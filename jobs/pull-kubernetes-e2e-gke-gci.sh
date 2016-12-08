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
if [[ "${PULL_BASE_REF:-}" == "release-1.0" || "${PULL_BASE_REF:-}" == "release-1.1" ]]; then
  echo "PR GCE job disabled for legacy branches."
  exit
fi

export KUBE_GCS_RELEASE_BUCKET=kubernetes-release-pull
export KUBE_GCS_RELEASE_SUFFIX="/${JOB_NAME}"
export KUBE_GCS_UPDATE_LATEST=n
export JENKINS_USE_LOCAL_BINARIES=y
export KUBE_FASTBUILD=true
./hack/jenkins/build.sh

# TODO(spxtr): once https://github.com/kubernetes/kubernetes/pull/35453 is in,
# remove the first branch here.
if [[ -e build/util.sh ]]; then
  version=$(source build/util.sh && echo $(kube::release::semantic_version))
elif [[ -e build-tools/util.sh ]]; then
  version=$(source build-tools/util.sh && echo $(kube::release::semantic_version))
else
  echo "Could not find build/util.sh or build-tools/util.sh." >&2
  exit 1
fi
gsutil -m rsync -r "gs://kubernetes-release-pull/ci/${JOB_NAME}/${version}" "gs://kubernetes-release-dev/ci/${version}-pull-gke-gci"
# Strip off the leading "v" from the cluster version.
export CLUSTER_API_VERSION="${version:1}-pull-gke-gci"

export KUBERNETES_PROVIDER="gke"
export E2E_MIN_STARTUP_PODS="1"

# Flake detection. Individual tests get a second chance to pass.
export GINKGO_TOLERATE_FLAKES="y"
export E2E_NAME="e2e-gke-${NODE_NAME}-${EXECUTOR_NUMBER:-0}"
export GINKGO_PARALLEL="y"

# Just run a smoke test.
export GINKGO_TEST_ARGS="--ginkgo.focus=Guestbook"
export FAIL_ON_GCP_RESOURCE_LEAK="false"
export PROJECT="k8s-jkns-pr-gci-gke"

# Since we're only running one test, just use two nodes.
export NUM_NODES="2"

# Assume we're upping, testing, and downing a cluster
export E2E_UP="true"
export E2E_TEST="true"
export E2E_DOWN="true"
export E2E_OPT="--check_version_skew=false"

# Skip gcloud update checking
export CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true

# GKE variables
export CLUSTER_NAME=${E2E_NAME}
export KUBE_GKE_NETWORK=${E2E_NAME}
export ZONE="us-central1-f"
export CLOUDSDK_API_ENDPOINT_OVERRIDES_CONTAINER="https://test-container.sandbox.googleapis.com/"
export CLOUDSDK_CONTAINER_USE_CLIENT_CERTIFICATE=False

# Get golang into our PATH so we can run e2e.go
export PATH=${PATH}:/usr/local/go/bin

readonly runner="${testinfra}/jenkins/dockerized-e2e-runner.sh"
export DOCKER_TIMEOUT="75m"
export KUBEKINS_TIMEOUT="55m"
"${runner}"
