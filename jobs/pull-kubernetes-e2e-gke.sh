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
if [[ "${ghprbTargetBranch:-}" == "release-1.0" || "${ghprbTargetBranch:-}" == "release-1.1" ]]; then
  echo "PR GCE job disabled for legacy branches."
  exit
fi

export KUBE_SKIP_PUSH_GCS=n
export KUBE_GCS_RELEASE_BUCKET=kubernetes-release-pull
export KUBE_RUN_FROM_OUTPUT=y
export KUBE_FASTBUILD=true
export KUBE_GCS_UPDATE_LATEST=n
./hack/jenkins/build.sh

version=$(source build/util.sh && echo $(kube::release::semantic_version))
gsutil -m rsync -r "gs://kubernetes-release-pull/ci/${version}" "gs://kubernetes-release-dev/ci/${version}-pull"
# Strip off the leading 'v' from the cluster version.
export CLUSTER_API_VERSION="${version:1}-pull"

export KUBERNETES_PROVIDER="gce"
export E2E_MIN_STARTUP_PODS="1"

# Flake detection. Individual tests get a second chance to pass.
export GINKGO_TOLERATE_FLAKES="y"
export E2E_NAME="e2e-gce-${NODE_NAME}-${EXECUTOR_NUMBER:-0}"
export GINKGO_PARALLEL="y"

# Just run a smoke test.
export GINKGO_TEST_ARGS="--ginkgo.focus=Guestbook"
export FAIL_ON_GCP_RESOURCE_LEAK="false"
export PROJECT="k8s-jkns-pr-gke"

# Since we're only running one test, just use two nodes.
export NUM_NODES="2"

# Assume we're upping, testing, and downing a cluster
export E2E_UP="true"
export E2E_TEST="true"
export E2E_DOWN="true"
export E2E_OPT="--check_version_skew=false"

# TODO(mtaufen): Change this to "gci" once we are confident
# that tests behave similarly enough on GCI.
export KUBE_GKE_IMAGE_TYPE="container_vm"

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
timeout -k 15m 55m "${runner}" && rc=$? || rc=$?
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
