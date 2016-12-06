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
export KUBERNETES_PROVIDER="gke"

### project-env
# expected empty

### job-env
export E2E_NAME="gke-large-deploy"
export PROJECT="kubernetes-scale"
# TODO: Remove FAIL_ON_GCP_RESOURCE_LEAK when PROJECT changes back to gke-large-cluster-jenkins.
export FAIL_ON_GCP_RESOURCE_LEAK="false"
export ZONE="us-east1-a"
export NUM_NODES=2000
export MACHINE_TYPE="n1-standard-1"
export HEAPSTER_MACHINE_TYPE="n1-standard-4"
export ALLOWED_NOTREADY_NODES="20"
# We were asked (by MIG team) to not create more than 5 MIGs per zone.
# We also paged SREs with max-nodes-per-pool=400 (5 concurrent MIGs)
# So setting max-nodes-per-pool=1000, to check if that helps.
export GKE_CREATE_FLAGS="--max-nodes-per-pool=1000"
export CLOUDSDK_CONTAINER_USE_CLIENT_CERTIFICATE=True
export CLOUDSDK_API_ENDPOINT_OVERRIDES_CONTAINER="https://staging-container.sandbox.googleapis.com/"
export KUBE_NODE_OS_DISTRIBUTION="debian"
export E2E_TEST="false"
export E2E_UP="false"

### post-env

# Assume we're upping, testing, and downing a cluster
export E2E_UP="${E2E_UP:-true}"
export E2E_TEST="${E2E_TEST:-true}"
export E2E_DOWN="${E2E_DOWN:-true}"

export E2E_NAME='bootstrap-e2e'

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
export DOCKER_TIMEOUT="200m"
export KUBEKINS_TIMEOUT="180m"
"${runner}"
