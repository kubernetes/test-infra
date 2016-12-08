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
export FAIL_ON_GCP_RESOURCE_LEAK="true"
export CLOUDSDK_CORE_PRINT_UNHANDLED_TRACEBACKS="1"
export KUBE_GCE_ZONE="us-central1-f"
export KUBE_GCS_RELEASE_BUCKET=kubernetes-federation-release
export KUBE_GCS_DEV_RELEASE_BUCKET=kubernetes-federation-release
export KUBE_NODE_OS_DISTRIBUTION="debian"

### soak-env
export JENKINS_SOAK_MODE="y"
export FAIL_ON_GCP_RESOURCE_LEAK="false"
export E2E_TEST="false"
export E2E_DOWN="false"

### job-env
export PROJECT="k8s-jkns-gce-federation-soak"
# Need the 8 essential kube-system pods ready before declaring cluster ready
# etcd-server, kube-apiserver, kube-controller-manager, kube-dns
# kube-scheduler, l7-default-backend, l7-lb-controller, kube-addon-manager
export GINKGO_TEST_ARGS="--ginkgo.focus=\[Feature:Federation\]"
export GINKGO_PARALLEL="n" # We don't have namespaces yet in federation apiserver, so we need to serialize
export FEDERATION="true"
export DNS_ZONE_NAME="soak.test-f8n.k8s.io."
export FEDERATIONS_DOMAIN_MAP="federation=soak.test-f8n.k8s.io"
export E2E_ZONES="us-central1-a us-central1-b us-central1-f" # Where the clusters will be created. Federation components are now deployed to the last one.
export FEDERATION_PUSH_REPO_BASE="gcr.io/k8s-jkns-gce-federation-soak"

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
export DOCKER_TIMEOUT="110m"
export KUBEKINS_TIMEOUT="90m"
"${runner}"
