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

### provider-env
export CLOUDSDK_API_ENDPOINT_OVERRIDES_CONTAINER="https://test-container.sandbox.googleapis.com/"
export CLOUDSDK_BUCKET="gs://cloud-sdk-testing/ci/staging"
export E2E_MIN_STARTUP_PODS="8"
export FAIL_ON_GCP_RESOURCE_LEAK="true"
export KUBERNETES_PROVIDER="gke"

### project-env
# expected empty

### job-env
readonly version_cluster="1.4"
readonly version_client="1.3"
readonly version_infix="1-4-1-3"


export E2E_OPT="--check_version_skew=false"
export GINKGO_PARALLEL="y"
export GINKGO_TEST_ARGS="--ginkgo.focus=Kubectl"
export JENKINS_PUBLISHED_SKEW_VERSION="ci/latest-${version_client}"
export JENKINS_PUBLISHED_VERSION="ci/latest-${version_cluster}"
[ "${version_client}" = "latest" ] && JENKINS_PUBLISHED_SKEW_VERSION="ci/latest"
[ "${version_cluster}" = "latest" ] && JENKINS_PUBLISHED_VERSION="ci/latest"
export PROJECT="kube-gke-upg-1-4-1-3-ctl-skew"
export KUBE_GKE_IMAGE_TYPE="container_vm"
export ZONE="us-central1-c"

### version-env
export JENKINS_USE_SKEW_TESTS="true"

### post-env

# Assume we're upping, testing, and downing a cluster
export E2E_UP="${E2E_UP:-true}"
export E2E_TEST="${E2E_TEST:-true}"
export E2E_DOWN="${E2E_DOWN:-true}"

export E2E_NAME="bootstrap-e2e"

# Skip gcloud update checking
export CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true
# Use default component update behavior
export CLOUDSDK_EXPERIMENTAL_FAST_COMPONENT_UPDATE=false

# AWS variables
export KUBE_AWS_INSTANCE_PREFIX="${E2E_NAME}"

# GCE variables
export INSTANCE_PREFIX="${E2E_NAME}"
export KUBE_GCE_NETWORK="${E2E_NAME}"
export KUBE_GCE_INSTANCE_PREFIX="${E2E_NAME}"

# GKE variables
export CLUSTER_NAME="${E2E_NAME}"
export KUBE_GKE_NETWORK="${E2E_NAME}"

# Get golang into our PATH so we can run e2e.go
export PATH="${PATH}:/usr/local/go/bin"

### Runner
readonly runner="${testinfra}/jenkins/dockerized-e2e-runner.sh"
export DOCKER_TIMEOUT="140m"
export KUBEKINS_TIMEOUT="120m"
"${runner}"
