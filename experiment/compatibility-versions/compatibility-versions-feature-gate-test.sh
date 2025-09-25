#!/usr/bin/env bash
# Copyright 2025 The Kubernetes Authors.
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

# hack script for running kind clusters, fetching kube-apiserver metrics, and validating feature gates
# must be run with a kubernetes checkout in $PWD (IE from the checkout)
# Usage: compatibility-versions-feature-gate-test.sh

set -o errexit -o nounset -o pipefail
set -o xtrace

# Settings:
# GA_ONLY: true  - limit to GA APIs/features as much as possible
#          false - (default) APIs and features left at defaults

# FEATURE_GATES:
#          JSON or YAML encoding of a string/bool map: {"FeatureGateA": true, "FeatureGateB": false}
#          Enables or disables feature gates in the entire cluster.
#          Cannot be used when GA_ONLY=true.

# RUNTIME_CONFIG:
#          JSON or YAML encoding of a string/string (!) map: {"apia.example.com/v1alpha1": "true", "apib.example.com/v1beta1": "false"}
#          Enables API groups in the apiserver via --runtime-config.
#          Cannot be used when GA_ONLY=true.

# cleanup logic for cleanup on exit
CLEANED_UP=false
cleanup() {
  if [ "$CLEANED_UP" = "true" ]; then
    return
  fi
  # KIND_CREATE_ATTEMPTED is true once we: kind create
  if [ "${KIND_CREATE_ATTEMPTED:-}" = true ]; then
    kind "export" logs "${ARTIFACTS}" || true
    kind delete cluster || true
  fi
  rm -f _output/bin/kubectl || true
  # remove our tempdir, this needs to be last, or it will prevent kind delete
  if [ -n "${TMP_DIR:-}" ]; then
    rm -rf "${TMP_DIR:?}"
  fi
  CLEANED_UP=true
}

# setup signal handlers
# shellcheck disable=SC2317 # this is not unreachable code
signal_handler() {
  cleanup
}
trap signal_handler INT TERM

# build kubernetes / node image, kubectl binary
build() {
  # build the node image w/ kubernetes
  kind build node-image -v 1
  # make sure we have kubectl
  make all WHAT="cmd/kubectl"

  # Ensure the built kubectl is used instead of system
  export PATH="${PWD}/_output/bin:$PATH"
}

check_structured_log_support() {
	case "${KUBE_VERSION}" in
		v1.1[0-8].*)
			echo "$1 is only supported on versions >= v1.19, got ${KUBE_VERSION}"
			exit 1
			;;
	esac
}

# up a cluster with kind
create_cluster() {
  # Grab the version of the cluster we're about to start
  KUBE_VERSION="$(docker run --rm --entrypoint=cat "kindest/node:latest" /kind/version)"

  # Default Log level for all components in test clusters
  KIND_CLUSTER_LOG_LEVEL=${KIND_CLUSTER_LOG_LEVEL:-4}

  EMULATED_VERSION=${EMULATED_VERSION:-}

  # potentially enable --logging-format
  CLUSTER_LOG_FORMAT=${CLUSTER_LOG_FORMAT:-}
  scheduler_extra_args="      \"v\": \"${KIND_CLUSTER_LOG_LEVEL}\""
  controllerManager_extra_args="      \"v\": \"${KIND_CLUSTER_LOG_LEVEL}\""
  apiServer_extra_args="      \"v\": \"${KIND_CLUSTER_LOG_LEVEL}\""
  kubelet_extra_args="      \"v\": \"${KIND_CLUSTER_LOG_LEVEL}\""

  if [ -n "$CLUSTER_LOG_FORMAT" ]; then
      check_structured_log_support "CLUSTER_LOG_FORMAT"
      scheduler_extra_args="${scheduler_extra_args}
      \"logging-format\": \"${CLUSTER_LOG_FORMAT}\""
      controllerManager_extra_args="${controllerManager_extra_args}
      \"logging-format\": \"${CLUSTER_LOG_FORMAT}\""
      apiServer_extra_args="${apiServer_extra_args}
      \"logging-format\": \"${CLUSTER_LOG_FORMAT}\""
  fi

  KUBELET_LOG_FORMAT=${KUBELET_LOG_FORMAT:-$CLUSTER_LOG_FORMAT}
  if [ -n "$KUBELET_LOG_FORMAT" ]; then
      check_structured_log_support "KUBECTL_LOG_FORMAT"
      kubelet_extra_args="${kubelet_extra_args}
      \"logging-format\": \"${KUBELET_LOG_FORMAT}\""
  fi

  # JSON or YAML map injected into featureGates config
  feature_gates="${FEATURE_GATES:-{\}}"
  # --runtime-config argument value passed to the API server, again as a map
  runtime_config="${RUNTIME_CONFIG:-{\}}"

  case "${GA_ONLY:-false}" in
  false)
    :
    ;;
  true)
    if [ "${feature_gates}" != "{}" ]; then
      echo "GA_ONLY=true and FEATURE_GATES=${feature_gates} are mutually exclusive."
      exit 1
    fi
    if [ "${runtime_config}" != "{}" ]; then
      echo "GA_ONLY=true and RUNTIME_CONFIG=${runtime_config} are mutually exclusive."
      exit 1
    fi

    echo "Limiting to GA APIs and features for ${KUBE_VERSION}"
    feature_gates='{"AllAlpha":false,"AllBeta":false}'
    runtime_config='{"api/alpha":"false", "api/beta":"false"}'
    ;;
  *)
    echo "\$GA_ONLY set to '${GA_ONLY}'; supported values are true and false (default)"
    exit 1
    ;;
  esac

  # create the config file
  cat <<EOF > "${ARTIFACTS}/kind-config.yaml"
# config for 1 control plane node and 2 workers (necessary for conformance)
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  ipFamily: ${IP_FAMILY:-ipv4}
  kubeProxyMode: ${KUBE_PROXY_MODE:-iptables}
  # don't pass through host search paths
  # TODO: possibly a reasonable default in the future for kind ...
  dnsSearch: []
nodes:
- role: control-plane
featureGates: ${feature_gates}
runtimeConfig: ${runtime_config}
kubeadmConfigPatches:
- |
  kind: ClusterConfiguration
  metadata:
    name: config
  apiServer:
    extraArgs:
${apiServer_extra_args}
      "emulated-version": "${EMULATED_VERSION}"
  controllerManager:
    extraArgs:
${controllerManager_extra_args}
      "emulated-version": "${EMULATED_VERSION}"
  scheduler:
    extraArgs:
${scheduler_extra_args}
      "emulated-version": "${EMULATED_VERSION}"
  ---
  kind: InitConfiguration
  nodeRegistration:
    kubeletExtraArgs:
${kubelet_extra_args}
  ---
  kind: JoinConfiguration
  nodeRegistration:
    kubeletExtraArgs:
${kubelet_extra_args}
EOF

  KIND_CREATE_ATTEMPTED=true
  kind create cluster \
    --image=kindest/node:latest \
    --retain \
    --wait=1m \
    -v=3 \
    "--config=${ARTIFACTS}/kind-config.yaml"

  # debug cluster version
  kubectl version

  # Patch kube-proxy to set the verbosity level
  kubectl patch -n kube-system daemonset/kube-proxy \
    --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/command/-", "value": "--v='"${KIND_CLUSTER_LOG_LEVEL}"'" }]'
}

fetch_metrics() {
  local output_file="$1"
  echo "Fetching metrics to ${output_file}..."
  kubectl get --raw /metrics > "${output_file}"
}

main() {
  TMP_DIR=$(mktemp -d)
  export ARTIFACTS="${ARTIFACTS:-${PWD}/_artifacts}"
  mkdir -p "${ARTIFACTS}"

  # Get current and n-1 version numbers
  WORKSPACE_STATUS=$(./hack/print-workspace-status.sh)
  MAJOR_VERSION=$(echo "$WORKSPACE_STATUS" | awk '/STABLE_BUILD_MAJOR_VERSION/ {print $2}')
  MINOR_VERSION=$(echo "$WORKSPACE_STATUS" | awk '/STABLE_BUILD_MINOR_VERSION/ {split($2, minor, "+"); print minor[1]}')
  export VERSION_DELTA=${VERSION_DELTA:-1}

  export CURRENT_VERSION="${MAJOR_VERSION}.${MINOR_VERSION}"
  export EMULATED_VERSION=$(get_release_version)

  # Check if gitVersion contains alpha.0 and increment VERSION_DELTA if needed
  # If the current version is alpha.0, it means the previous *stable* or developed
  # branch is actually n-2 relative to the current minor number for compatibility purposes.
  GIT_VERSION=$(echo "$WORKSPACE_STATUS" | awk '/^gitVersion / {print $2}')
  if [[ "${GIT_VERSION}" == *alpha.0* ]]; then
    echo "Detected alpha.0 in gitVersion (${GIT_VERSION}), treating as still the previous minor version."
    VERSION_DELTA=$((VERSION_DELTA + 1))
    echo "Adjusted VERSION_DELTA: ${VERSION_DELTA}"
  fi

  # Set original paths with fallbacks
  export VERSIONED_FEATURE_LIST=${VERSIONED_FEATURE_LIST:-"test/featuregates_linter/test_data/versioned_feature_list.yaml"}
  export PREV_VERSIONED_FEATURE_LIST=${PREV_VERSIONED_FEATURE_LIST:-"release-${EMULATED_VERSION}/test/featuregates_linter/test_data/versioned_feature_list.yaml"}

  # Create and validate previous cluster
  git clone --filter=blob:none --single-branch --branch "release-${EMULATED_VERSION}" https://github.com/kubernetes/kubernetes.git "release-${EMULATED_VERSION}"

  # Build current version
  build

  # Create and validate latest cluster
  KUBECONFIG="${HOME}/.kube/kind-test-config-latest"
  export KUBECONFIG
  create_cluster
  LATEST_METRICS="${ARTIFACTS}/latest_metrics.txt"
  fetch_metrics "${LATEST_METRICS}"
  LATEST_RESULTS="${ARTIFACTS}/latest_results.txt"
  
  # Check if files exist at the current paths and update if needed with alternative paths
  if [ ! -f "$VERSIONED_FEATURE_LIST" ]; then
    alt_path="test/compatibility_lifecycle/reference/versioned_feature_list.yaml"
    if [ -f "$alt_path" ]; then
      export VERSIONED_FEATURE_LIST="$alt_path"
      echo "Using alternative path for VERSIONED_FEATURE_LIST: $alt_path"
    fi
  fi

  if [ ! -f "$PREV_VERSIONED_FEATURE_LIST" ]; then
    alt_path="release-${EMULATED_VERSION}/test/compatibility_lifecycle/reference/versioned_feature_list.yaml"
    if [ -f "$alt_path" ]; then
      export PREV_VERSIONED_FEATURE_LIST="$alt_path"
      echo "Using alternative path for PREV_VERSIONED_FEATURE_LIST: $alt_path"
    fi
  fi


  VALIDATE_SCRIPT="${VALIDATE_SCRIPT:-${PWD}/../test-infra/experiment/compatibility-versions/validate-compatibility-versions-feature-gates.sh}"
  "${VALIDATE_SCRIPT}" "${EMULATED_VERSION}" "${CURRENT_VERSION}" "${LATEST_METRICS}" "${VERSIONED_FEATURE_LIST}" "${PREV_VERSIONED_FEATURE_LIST}" "${LATEST_RESULTS}"

  # Report results
  echo "=== Latest Cluster (${EMULATED_VERSION}) Validation ==="
  cat "${LATEST_RESULTS}"

  if grep -q "FAIL" "${LATEST_RESULTS}"; then
    echo "Validation failures detected"
    exit 1
  fi

  cleanup
}

get_release_version() {
  git ls-remote --heads https://github.com/kubernetes/kubernetes.git | \
    grep -o 'release-[0-9]\+\.[0-9]\+' | \
    sort -t. -k1,1n -k2,2n | \
    tail -n $VERSION_DELTA | \
    head -n1 | \
    cut -d- -f2
}

main