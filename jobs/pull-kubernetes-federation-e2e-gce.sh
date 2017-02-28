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

export FEDERATION="true"
export USE_KUBEFED="true"

export PROJECT="${PROJECT:-k8s-jkns-pr-bldr-e2e-gce-fdrtn}"
export KUBE_REGISTRY="gcr.io/k8s-jkns-pr-bldr-e2e-gce-fdrtn"
export KUBERNETES_PROVIDER="gce"

# Build federation images. This doesn't build kubernetes images.
export JENKINS_USE_LOCAL_BINARIES=y
./hack/jenkins/build-federation.sh

# Recycle control plane.
# We only recycle federation control plane in each run, we don't
# want to recycle the clusters, it's too slow.
# This is accomplished by not setting FEDERATION_CLUSTERS env
# var or setting it to empty string. We set it to empty string
# to explicitly call it out.
export FEDERATION_CLUSTERS=""

# Federation control plane options.
export DNS_ZONE_NAME="pr-bldr.test-f8n.k8s.io."
export FEDERATIONS_DOMAIN_MAP="federation=pr-bldr.test-f8n.k8s.io"

# This is a shared variable that is used for both k8s and
# federation tests to indicate that the tests must be run.
export E2E_TEST="true"

# Ginkgo and other test arguments.
export GINKGO_TEST_ARGS="--ginkgo.focus=\[Feature:Federation\] --ginkgo.skip=\[Slow\]|\[Serial\]|\[Disruptive\]|\[Flaky\]|\[NoCluster\]"
export GINKGO_PARALLEL="y"
# Flake detection. Individual tests get a second chance to pass.
export GINKGO_TOLERATE_FLAKES="y"
export FAIL_ON_GCP_RESOURCE_LEAK="false"
export E2E_MIN_STARTUP_PODS="1"

# Panic if anything mutates a shared informer cache
export ENABLE_CACHE_MUTATION_DETECTOR="true"

# Misc environment configuration.
# Skip gcloud update checking
export CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true
# Get golang into our PATH so we can run e2e.go
export PATH=${PATH}:/usr/local/go/bin

readonly runner="${testinfra}/jenkins/dockerized-e2e-runner.sh"
export KUBEKINS_TIMEOUT="90m"
timeout -k 20m "${KUBEKINS_TIMEOUT}" "${runner}" && rc=$? || rc=$?
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
