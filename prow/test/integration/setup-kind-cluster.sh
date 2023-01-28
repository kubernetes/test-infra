#!/usr/bin/env bash
# Copyright 2022 The Kubernetes Authors.
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

# Set up the KIND cluster.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_ROOT}"/lib.sh

function usage() {
  >&2 cat <<EOF
Set up the KIND cluster. Ultimately, this results in the running of 2 docker containers, called kind-prow-integration-control-plane and kind-prow-integration-registry.

Usage: $0 [options]

Examples:
  # Setup the KIND cluster with default settings (this is what you want when you
  # are running this script manually for debugging).
  $0

  # Use a different node port (32222) for the KIND cluster than the default
  # 30303 one. This port is used by fakepubsub. Note that the port must be in
  # the range 30000-32767.
  $0 -fakepubsub-node-port=32222

Options:
    -fakepubsub-node-port='':
        Make the fakepubsub service use the provided node port (default 30303).

    -help:
        Display this help message.
EOF
}

function main() {
  local fakepubsub_node_port
  fakepubsub_node_port=30303
  # If we abort the setup script with Ctrl+C, delete the cluster because the
  # setup process was interrupted.
  # shellcheck disable=SC2064
  trap "${SCRIPT_ROOT}/teardown.sh -kind-cluster" SIGINT SIGTERM

  if [[ -z "${HOME:-}" ]]; then
    HOME="$(cd ~ && pwd -P)"
    export HOME
  fi

  for arg in "$@"; do
    case "${arg}" in
      -fakepubsub-node-port=*)
        fakepubsub_node_port="${arg#-fakepubsub-node-port=}"
        ;;
      -help)
        usage
        return
        ;;
      --*)
        echo >&2 "cannot use flags with two leading dashes ('--...'), use single dashes instead ('-...')"
        return 1
        ;;
    esac
  done

  # The KIND cluster is configured to use a special local docker registry; this
  # registry must exist before we bring up the cluster. See https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry for more information.
  setup_local_docker_registry

  # Required for some tests (e.g., horologium_test.go) that use a dummy image.
  #
  # TODO(listx): Move this code to horologium_test.go, as it is orthogonal to
  # KIND cluster setup.
  populate_registry gcr.io/k8s-prow/alpine:latest alpine:latest

  if cluster_running; then
    log "Using existing KIND cluster"
  else
    "${SCRIPT_ROOT}/teardown.sh" -kind-cluster
    create_cluster "${fakepubsub_node_port:-30303}"
  fi
  setup_cluster

  # Use nginx as a reverse proxy and load balancer for the cluster. We don't
  # wait for it here in this script because it can take a while to finish. We do
  # wait for it in the setup-prow-components.sh script.
  log "Installing nginx ingress controller on KIND cluster"
  do_kubectl apply -f "${SCRIPT_ROOT}/config/nginx.yaml"
}

function cluster_running() {
  local running
  running="$(docker inspect -f '{{.State.Running}}' "${_KIND_CLUSTER_NAME}-control-plane" 2>/dev/null || true)"
  [[ "${running}" == "true" ]]
}

# Create a cluster with the local registry enabled in containerd,
# as well as configure node-labels and extraPortMappings for ingress.
# See: https://kind.sigs.k8s.io/docs/user/ingress/#create-cluster.
function create_cluster() {
  log "Creating KIND cluster"

  local fakepubsub_node_port
  fakepubsub_node_port="${1:-30303}"

  cat <<EOF | kind create cluster --name "${_KIND_CLUSTER_NAME}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${LOCAL_DOCKER_REGISTRY_PORT}"]
    endpoint = ["http://${LOCAL_DOCKER_REGISTRY_NAME}:5000"]
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
  - containerPort: 32000
    hostPort: 32000
    protocol: TCP
  - containerPort: ${fakepubsub_node_port}
    hostPort: ${fakepubsub_node_port}
    protocol: TCP
EOF

}

# Connect the registry to the cluster network.
function setup_cluster() {
  log "Setting up local registry for cluster"
  # Ignore the error, as the network may already be connected.
  docker network connect "kind" "${LOCAL_DOCKER_REGISTRY_NAME}" 2>/dev/null || true

  cat <<EOF | do_kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${LOCAL_DOCKER_REGISTRY_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

}

function setup_local_docker_registry() {
  # Create registry container unless it already exists.
  running="$(docker inspect -f '{{.State.Running}}' "${LOCAL_DOCKER_REGISTRY_NAME}" 2>/dev/null || true)"
  if [[ "${running}" == 'true' ]]; then
    log "Local registry localhost:${LOCAL_DOCKER_REGISTRY_PORT} already exists"
  else
    log "Creating docker container for hosting local registry localhost:${LOCAL_DOCKER_REGISTRY_PORT}"
    "${SCRIPT_ROOT}/teardown.sh" -local-registry
    docker run \
      -d --restart=always -p "127.0.0.1:${LOCAL_DOCKER_REGISTRY_PORT}:5000" --name "${LOCAL_DOCKER_REGISTRY_NAME}" \
      registry:2
  fi
}

function populate_registry() {
  local src
  local dest

  src="${1:-}"
  dest="${2:-}"
  dest="localhost:${LOCAL_DOCKER_REGISTRY_PORT}/${dest}"
  log "Push ${src} to registry as ${2}"
  docker pull "${src}"
  docker tag "${src}" "${dest}"
  docker push "${dest}"
}

main "$@"
