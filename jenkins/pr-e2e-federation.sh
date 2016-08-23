#!/bin/bash
# Copyright 2016 The Kubernetes Authors All rights reserved.
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

# Federation specific params
export FEDERATION="true"
export PROJECT="k8s-jkns-pr-bldr-e2e-gce-fdrtn"
export FEDERATION_PUSH_REPO_BASE="gcr.io/k8s-jkns-pr-bldr-e2e-gce-fdrtn"
export GINKGO_PARALLEL="n" # We don't have namespaces yet in federation apiserver, so we need to serialize
export GINKGO_TEST_ARGS="--ginkgo.focus=\[Feature:Federation\]"
export E2E_ZONES="us-central1-a us-central1-f" # Where the clusters will be created. Federation components are now deployed to the last one.
export KUBE_GCE_ZONE="us-central1-f" #TODO(colhom): This should be generalized out to plural case
export DNS_ZONE_NAME="k8s-federation.com."
export FEDERATIONS_DOMAIN_MAP="federation=k8s-federation.com"

export KUBE_SKIP_PUSH_GCS=y
export KUBE_RUN_FROM_OUTPUT=y
export KUBE_FASTBUILD=true
./hack/jenkins/build.sh
# Nothing should want Jenkins $HOME
export HOME=${WORKSPACE}

export KUBERNETES_PROVIDER="gce"
export E2E_MIN_STARTUP_PODS="1"
export FAIL_ON_GCP_RESOURCE_LEAK="true"

# Flake detection. Individual tests get a second chance to pass.
export GINKGO_TOLERATE_FLAKES="y"

export E2E_NAME="fed-e2e-${NODE_NAME}-${EXECUTOR_NUMBER}"
export FAIL_ON_GCP_RESOURCE_LEAK="false"
export NUM_NODES="3"

# Force to use container-vm.
export KUBE_NODE_OS_DISTRIBUTION="debian"
# Assume we're upping, testing, and downing a cluster
export E2E_UP="true"
export E2E_TEST="true"
export E2E_DOWN="true"

# Skip gcloud update checking
export CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true

# GCE variables
export INSTANCE_PREFIX=${E2E_NAME}
export KUBE_GCE_NETWORK=${E2E_NAME}
export KUBE_GCE_INSTANCE_PREFIX=${E2E_NAME}

# Get golang into our PATH so we can run e2e.go
export PATH=${PATH}:/usr/local/go/bin


timeout -k 15m 55m {runner} && rc=$? || rc=$?
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

