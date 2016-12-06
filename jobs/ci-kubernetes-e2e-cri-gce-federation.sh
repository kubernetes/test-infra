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
export E2E_MIN_STARTUP_PODS="8"
export KUBE_GCE_ZONE="us-central1-f"
export FAIL_ON_GCP_RESOURCE_LEAK="true"
export CLOUDSDK_CORE_PRINT_UNHANDLED_TRACEBACKS="1"

### project-env
# expected empty

### job-env
export KUBELET_TEST_ARGS="--experimental-cri=true"
export KUBE_FEATURE_GATES="StreamingProxyRedirects=true"

export PROJECT="k8s-jkns-e2e-gce-cri-federatio" # GCE project IDs are restricted to 30 characters, so this name is intentionally truncated.
export GINKGO_TEST_ARGS="--ginkgo.focus=\[Feature:Federation\]"
export GINKGO_PARALLEL="n" # We don't have namespaces yet in federation apiserver, so we need to serialize
export FEDERATION="true"
export DNS_ZONE_NAME="gci.test-f8n.k8s.io."
export FEDERATIONS_DOMAIN_MAP="federation=gci.test-f8n.k8s.io"
export E2E_ZONES="us-central1-a us-central1-b us-central1-f" # Where the clusters will be created. Federation components are now deployed to the last one.
export FEDERATION_PUSH_REPO_BASE="gcr.io/k8s-jkns-e2e-gce-federation"
export KUBE_GCE_ZONE="us-central1-f" #TODO(colhom): This should be generalized out to plural case
export KUBE_GCS_RELEASE_BUCKET=kubernetes-federation-release
export KUBE_GCS_DEV_RELEASE_BUCKET=kubernetes-federation-release
export KUBE_OS_DISTRIBUTION="gci"

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
export DOCKER_TIMEOUT="915m"
export DOCKER_TIMEOUT="920m"
export KUBEKINS_TIMEOUT="900m"
"${runner}"
