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
export KUBERNETES_PROVIDER="gce"
export CLOUDSDK_CORE_PRINT_UNHANDLED_TRACEBACKS="1"

### project-env
# XXX Not a unique project
export PROJECT="kubernetes-scale"
export FAIL_ON_GCP_RESOURCE_LEAK="false"
# Override GCE defaults.
# Temporarily switch of Heapster, as this will not schedule anywhere.
# TODO: Think of a solution to enable it.
export KUBE_ENABLE_CLUSTER_MONITORING="none"
# TODO: Move to us-central1-c once we have permission for it.
export KUBE_GCE_ZONE="us-east1-a"
export MASTER_SIZE="n1-standard-32"
# Increase disk size to check if that helps for etcd latency.
export MASTER_DISK_SIZE="100GB"
export NODE_SIZE="n1-standard-1"
export NODE_DISK_SIZE="50GB"
# Reduce logs verbosity
export TEST_CLUSTER_LOG_LEVEL="--v=1"
# Switch off image puller to workaround #32191.
export PREPULL_E2E_IMAGES="false"
export MAX_INSTANCES_PER_MIG="1000"
# Increase resync period to simulate production
export TEST_CLUSTER_RESYNC_PERIOD="--min-resync-period=12h"
# Increase delete collection parallelism
export TEST_CLUSTER_DELETE_COLLECTION_WORKERS="--delete-collection-workers=16"
# =========================================
# Configuration we are targetting in 1.5
export TEST_ETCD_IMAGE="3.0.14-experimental.1"
export TEST_ETCD_VERSION="3.0.14"
export STORAGE_BACKEND="etcd3"
export TEST_CLUSTER_STORAGE_CONTENT_TYPE="--storage-media-type=application/vnd.kubernetes.protobuf"
export KUBE_NODE_OS_DISTRIBUTION="gci"

### job-env
export E2E_NAME="e2e-enormous-deploy"
export CLUSTER_IP_RANGE="10.224.0.0/13"
export NUM_NODES="2000"
export ALLOWED_NOTREADY_NODES="20"
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
