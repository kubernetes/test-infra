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

# hack script for running a kind e2e
# must be run with a kubernetes checkout in $PWD (IE from the checkout)
# Usage: SKIP="ginkgo skip regex" FOCUS="ginkgo focus regex" kind-e2e.sh

# common.sh
# This file contains shared functions and variables for kind e2e scripts


set -o errexit -o nounset -o xtrace

CLUSTER_NAME=${CLUSTER_NAME:-kind}
CONTROL_PLANE_COMPONENTS="kube-apiserver kube-controller-manager kube-scheduler"

# Settings:
# SKIP: ginkgo skip regex
# FOCUS: ginkgo focus regex
# LABEL_FILTER: ginkgo label query for selecting tests (see "Spec Labels" in https://onsi.github.io/ginkgo/#filtering-specs)
#
# The default is to focus on conformance tests. Serial tests get skipped when
# parallel testing is enabled. Using LABEL_FILTER instead of combining SKIP and
# FOCUS is recommended (more expressive, easier to read than regexp).
#
# FEATURE_GATES:
#          JSON or YAML encoding of a string/bool map: {"FeatureGateA": true, "FeatureGateB": false}
#          Enables or disables feature gates in the entire cluster.
# RUNTIME_CONFIG:
#          JSON or YAML encoding of a string/string (!) map: {"apia.example.com/v1alpha1": "true", "apib.example.com/v1beta1": "false"}
#          Enables API groups in the apiserver via --runtime-config.

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
  rm -f _output/bin/e2e.test || true
  # remove our tempdir, this needs to be last, or it will prevent kind delete
  if [ -n "${TMP_DIR:-}" ]; then
    rm -rf "${TMP_DIR:?}"
  fi
  CLEANED_UP=true
}

check_structured_log_support() {
	case "${KUBE_VERSION}" in
		v1.1[0-8].*)
			echo "$1 is only supported on versions >= v1.19, got ${KUBE_VERSION}"
			return 1
			;;
	esac
  return 0
}

# Function to check if version is greater than or equal to the target
version_gte() {
  local version=$1
  local target=$2
  
  # Extract major and minor versions
  local version_major version_minor
  version_major=$(echo "$version" | cut -d. -f1)
  version_minor=$(echo "$version" | cut -d. -f2)
  
  local target_major target_minor
  target_major=$(echo "$target" | cut -d. -f1)
  target_minor=$(echo "$target" | cut -d. -f2)
  
  # Compare major version first, then minor
  if [ "$version_major" -gt "$target_major" ]; then
    return 0
  elif [ "$version_major" -eq "$target_major" ] && [ "$version_minor" -ge "$target_minor" ]; then
    return 0
  fi
  return 1
}

# up a cluster with kind
create_cluster() {
  # Default Log level for all components in test clusters
  KIND_CLUSTER_LOG_LEVEL=${KIND_CLUSTER_LOG_LEVEL:-4}

  EMULATED_VERSION=${EMULATED_VERSION:-}
  export LATEST_VERSION="${LATEST_VERSION:-$(curl -Ls https://dl.k8s.io/ci/latest.txt)}"
  echo "{\"revision\":\"$LATEST_VERSION\"}" >"${ARTIFACTS}/metadata.json"

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
- role: worker
- role: worker
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
  # NOTE: must match the number of workers above
  NUM_NODES=2
  # actually create the cluster
  # TODO(BenTheElder): settle on verbosity for this script
  KIND_CREATE_ATTEMPTED=true
  kind create cluster \
    --image="kindest/node:v${PREV_VERSION}.0" \
    --retain \
    --wait=1m \
    -v=3 \
    "--config=${ARTIFACTS}/kind-config.yaml"

  # debug cluster version
  kubectl version

  # Patch kube-proxy to set the verbosity level
  kubectl patch -n kube-system daemonset/kube-proxy \
    --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/command/-", "value": "--v='"${KIND_CLUSTER_LOG_LEVEL}"'" }]'
  return 0
}

build_test_bins() {
  local release_branch=$1
  GINKGO_SRC_DIR="vendor/github.com/onsi/ginkgo/v2/ginkgo"

  echo "Building e2e.test binary from release branch ${release_branch}..."
  make all WHAT="cmd/kubectl test/e2e/e2e.test ${GINKGO_SRC_DIR}"

  # Ensure the built kubectl is used instead of system
  export PATH="${PWD}/_output/bin:$PATH"
  echo "Finished building e2e.test binary from ${release_branch}."
}

# TODO: Support Mac as a platform along with Linux https://github.com/kubernetes/test-infra/pull/34930#discussion_r2138760351
download_release_version_bins() {
  local version=$1
  curl -LO https://dl.k8s.io/ci/$(curl -Ls https://dl.k8s.io/ci/latest-${version}.txt)/kubernetes-test-$(go env GOOS)-$(go env GOARCH).tar.gz
  if [ $? -ne 0 ]; then
    echo "failed to download previous version ${version} binaries"
    return 1
  fi
  tar -xf kubernetes-test-$(go env GOOS)-$(go env GOARCH).tar.gz 
  mkdir -p _output/bin
  mv kubernetes/test/bin/* _output/bin
  export PATH="${PWD}/_output/bin:$PATH"
  return 0
}

download_current_version_bins() {
  curl -LO 'https://dl.k8s.io/ci/'"${1}"/'kubernetes-test-'"$(go env GOOS)-$(go env GOARCH)"'.tar.gz'
  if [ $? -ne 0 ]; then
    echo "failed to download current version binaries"
    return 1
  fi
  tar -xf kubernetes-test-$(go env GOOS)-$(go env GOARCH).tar.gz
  mkdir -p _output/bin
  mv kubernetes/test/bin/* _output/bin
  return 0
}

# run e2es with ginkgo-e2e.sh
run_e2e_tests() {
  # IPv6 clusters need some CoreDNS changes in order to work in k8s CI:
  # 1. k8s CI doesn´t offer IPv6 connectivity, so CoreDNS should be configured
  # to work in an offline environment:
  # https://github.com/coredns/coredns/issues/2494#issuecomment-457215452
  # 2. k8s CI adds following domains to resolv.conf search field:
  # c.k8s-prow-builds.internal google.internal.
  # CoreDNS should handle those domains and answer with NXDOMAIN instead of SERVFAIL
  # otherwise pods stops trying to resolve the domain.
  if [ "${IP_FAMILY:-ipv4}" = "ipv6" ]; then
    # Get the current config
    original_coredns=$(kubectl get -oyaml -n=kube-system configmap/coredns)
    echo "Original CoreDNS config:"
    echo "${original_coredns}"
    # Patch it
    fixed_coredns=$(
      printf '%s' "${original_coredns}" | sed \
        -e 's/^.*kubernetes cluster\.local/& internal/' \
        -e '/^.*upstream$/d' \
        -e '/^.*fallthrough.*$/d' \
        -e '/^.*forward . \/etc\/resolv.conf$/d' \
        -e '/^.*loop$/d' \
    )
    echo "Patched CoreDNS config:"
    echo "${fixed_coredns}"
    printf '%s' "${fixed_coredns}" | kubectl apply -f -
  fi

  # ginkgo regexes and label filter
  SKIP="${SKIP:-}"
  FOCUS="${FOCUS:-}"
  LABEL_FILTER="${LABEL_FILTER:-}"
  if [ -z "${FOCUS}" ] && [ -z "${LABEL_FILTER}" ]; then
    FOCUS="\\[Conformance\\]"
  fi
  # if we set PARALLEL=true, skip serial tests set --ginkgo-parallel
  if [ "${PARALLEL:-false}" = "true" ]; then
    export GINKGO_PARALLEL=y
    if [ -z "${SKIP}" ]; then
      SKIP="\\[Serial\\]"
    else
      SKIP="\\[Serial\\]|${SKIP}"
    fi
  fi

  # setting this env prevents ginkgo e2e from trying to run provider setup
  export KUBERNETES_CONFORMANCE_TEST='y'
  # setting these is required to make RuntimeClass tests work ... :/
  export KUBE_CONTAINER_RUNTIME=remote
  export KUBE_CONTAINER_RUNTIME_ENDPOINT=unix:///run/containerd/containerd.sock
  export KUBE_CONTAINER_RUNTIME_NAME=containerd
  # Setting some formatting vars for ginkgo
  export GINKGO_NO_COLOR=${GINKGO_NO_COLOR:-$(if [ -t 2 ]; then echo n; else echo y; fi)}
  # ginkgo can take forever to exit, so we run it in the background and save the
  # PID, bash will not run traps while waiting on a process, but it will while
  # running a builtin like `wait`, saving the PID also allows us to forward the
  # interrupt
  ginkgo \
    --nodes=8 \
    --focus="${FOCUS}" \
    --skip="${SKIP}" \
    --label-filter="${LABEL_FILTER}" \
    --silence-skips \
    --no-color \
    ./_output/bin/e2e.test -- \
    --provider=skeleton \
    --num-nodes="${NUM_NODES}" \
    --report-dir="${ARTIFACTS}" \
    --disable-log-dump=true &
  GINKGO_PID=$!
  wait "$GINKGO_PID"
}

upgrade_cluster_components() {
  # upgrade cluster components excluding the kubelet

  # Get the retry attempts, defaulting to 5 if not set
  RETRY_ATTEMPTS="${RETRY_ATTEMPTS:-5}"

  local attempt=1
  local success=false

  bash "${UPGRADE_SCRIPT}" --no-kubelet | tee "${ARTIFACTS}/upgrade-output-1.txt"
  bash "${UPGRADE_SCRIPT}" --no-kubelet | tee "${ARTIFACTS}/upgrade-output-2.txt"
  # Run the script twice, is necessary for fully updating the binaries

  while [ "$attempt" -le "$RETRY_ATTEMPTS" ]; do
    echo "Attempt $attempt of $RETRY_ATTEMPTS to upgrade cluster..."
    # Check if kubectl version reports the current version
    kind export kubeconfig --name kind
    if kubectl version | grep "Server Version:"| grep -q "$CURRENT_VERSION"; then
      echo "Upgrade successful! kubectl version reports $CURRENT_VERSION"
      success=true
      break  # Exit the loop on success
    fi

    attempt=$((attempt + 1))
    echo "Upgrade check $attempt failed. Retrying in 60s..."
    sleep 60
  done

  if ! "$success"; then
    echo "Upgrade failed after $RETRY_ATTEMPTS attempts."
    return 1
  fi
  return 0
}

set_emulation_version() {
  CONTROL_PLANE_NODES=$(kind get nodes --name ${CLUSTER_NAME} | grep control)
  local version=$1
  for n in $CONTROL_PLANE_NODES; do
    for i in $CONTROL_PLANE_COMPONENTS; do
      docker exec $n sed -e 's/emulated-version=.*/emulated-version='"${version}"'/' -i /etc/kubernetes/manifests/$i.yaml
      echo "Updated emulated version for component $i on node $n to $version"
      ret=0
      wait_for_container_running $n $i || ret=$?
      if [[ "$ret" -ne 0 ]]; then
        return $ret;
      fi
    done
  done
  return 0
}

delete_emulation_version() {
  CONTROL_PLANE_NODES=$(kind get nodes --name ${CLUSTER_NAME} | grep control)
  for n in $CONTROL_PLANE_NODES; do
    for i in $CONTROL_PLANE_COMPONENTS; do
      docker exec $n sed -e '/emulated-version=/d' -i /etc/kubernetes/manifests/$i.yaml
      docker exec $n sed -e '/emulation-forward-compatible=/d' -i /etc/kubernetes/manifests/$i.yaml
      echo "Deleted emulated version for component $i on node $n"
      ret=0
      wait_for_container_running $n $i || ret=$?
      if [[ "$ret" -ne 0 ]]; then
        return $ret;
      fi
    done
  done
  return 0
}

wait_for_container_running() {
  local node=$1
  local name=$2

  local attempt=1
  local success=false
  echo "Waiting for $name to become ready"
  while [ "$attempt" -le $RETRY_ATTEMPTS ]; do
    local container_id=$(docker exec $node crictl ps --output json | jq -r --arg NAME "$name" '.containers[] | select(.metadata.name == $NAME) | .id')
    if docker exec $node crictl ps --id $container_id | grep -q Running; then
      echo "Container $name is ready"
      success=true
      break
    fi
    attempt=$((attempt + 1))
    sleep 15
  done
  if ! "$success"; then
    echo "Container wait failed after $RETRY_ATTEMPTS attempts."
    return 1
  fi
  return 0
}