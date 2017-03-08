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

### builder

### job-env

export PROJECT="k8s-jkns-e2e-kubeadm-gce-ci"
export KUBERNETES_PROVIDER=kubernetes-anywhere
export E2E_NAME="e2e-kubeadm-gce"
export E2E_OPT="--deployment kubernetes-anywhere --kubernetes-anywhere-path /workspace/kubernetes-anywhere"
export E2E_OPT+=" --kubernetes-anywhere-phase2-provider kubeadm --kubernetes-anywhere-cluster ${E2E_NAME}"
export GINKGO_TEST_ARGS="--ginkgo.focus=\[Conformance\]"

### post-env

# Assume we're upping, testing, and downing a cluster
export E2E_UP="${E2E_UP:-true}"
# TODO(pipejakob): Reenable testing when we have a pod network that works with
#     kubeadm 1.6 clusters. For now, simply bringing up a cluster is a good e2e
#     test, since it will only succeed if the apiserver is healthy and all
#     expected nodes are registered.
export E2E_TEST="false"
export E2E_DOWN="${E2E_DOWN:-true}"

# Skip gcloud update checking
export CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true
# Use default component update behavior
export CLOUDSDK_EXPERIMENTAL_FAST_COMPONENT_UPDATE=false

# Get golang into our PATH so we can run e2e.go
export PATH="${PATH}:/usr/local/go/bin"

# After post-env
export GINKGO_PARALLEL="y"

### Runner
readonly runner="/workspace/e2e-runner.sh"
export DOCKER_TIMEOUT="140m"
export KUBEKINS_TIMEOUT="120m"
"${runner}"
