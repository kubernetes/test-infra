#!/usr/bin/env bash
# Copyright 2020 The Kubernetes Authors.
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

CURRENT_DIR="$(cd "$(dirname "$0")" && pwd)"
CONFIG_ROOT_DIR="${CURRENT_DIR}"
if [[ -n "${1:-}" && "${1}" == "--config-path" ]]; then
  echo "Override CONFIG_ROOT_DIR"
  CONFIG_ROOT_DIR="${2:-}"
  echo "CONFIG_ROOT_DIR: ${CONFIG_ROOT_DIR}"
  shift 2
fi

readonly DEFAULT_CLUSTER_NAME="kind-prow-integration"
readonly DEFAULT_CONTEXT="kind-${DEFAULT_CLUSTER_NAME}"
readonly DEFAULT_REGISTRY_NAME="kind-registry"
readonly DEFAULT_REGISTRY_PORT="5000"
readonly PROW_COMPONENTS="sinker crier fakeghserver"

if [[ -z "${HOME:-}" ]]; then # kubectl looks for HOME which is not set in bazel
  export HOME="$(cd ~ && pwd -P)"
fi

function do_kubectl() {
  kubectl --context=${DEFAULT_CONTEXT} $@
}

# create a cluster with the local registry enabled in containerd,
# as well as configure node-lables and extraPortMappings for ingress,
# see: https://kind.sigs.k8s.io/docs/user/ingress/#create-cluster
function create_cluster_if_not_exist() {
  local running
  running="$(docker inspect -f '{{.State.Running}}' "${DEFAULT_CLUSTER_NAME}-control-plane" 2>/dev/null || true)"
  if [ "${running}" != 'true' ]; then
    echo "Create kind cluster"
    cat <<EOF | kind create cluster --name ${DEFAULT_CLUSTER_NAME} --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${DEFAULT_REGISTRY_PORT}"]
    endpoint = ["http://${DEFAULT_REGISTRY_NAME}:${DEFAULT_REGISTRY_PORT}"]
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
EOF
  else
    echo "Using existing kind cluster"
  fi
}

# connect the registry to the cluster network
function setup_cluster() {
  echo "Set up local registry for cluster"
  # ignore the error, as the network may already be connected
  docker network connect "kind" "${DEFAULT_REGISTRY_NAME}" 2>/dev/null || true

  echo "Document the local registry"
  # https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
  cat <<EOF | do_kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${DEFAULT_REGISTRY_PORT}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

  echo "Install nginx on kind cluster"
  # Pin the ingress-nginx manifest to 8b99f49d2d9c042355da9e53c2648bd0c049ae52 (Release 0.41.2) on 11/22/2020.
  do_kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/8b99f49d2d9c042355da9e53c2648bd0c049ae52/deploy/static/provider/kind/deploy.yaml
}

function deploy_prow() {
  echo "Remove previous installaion"
  for app in ${PROW_COMPONENTS}; do
    do_kubectl delete deployment -l app=${app}
    do_kubectl delete pods -l app=${app}
  done

  echo "Deploy prow components"
  # An unfortunately workaround for https://github.com/kubernetes/ingress-nginx/issues/5968.
  do_kubectl delete -A ValidatingWebhookConfiguration ingress-nginx-admission
  do_kubectl create configmap config --from-file=config.yaml=${CONFIG_ROOT_DIR}/config.yaml --dry-run -oyaml | kubectl apply -f -
  do_kubectl apply -f ${CONFIG_ROOT_DIR}/cluster

  echo "Wait until nginx is ready"
  for _ in 1 2 3; do
    if do_kubectl wait --namespace ingress-nginx \
      --for=condition=ready pod \
      --selector=app.kubernetes.io/component=controller \
      --timeout=180s; then
      break
    else
      sleep 5
    fi
  done
  echo "Nginx is ready"

  echo "Waiting for prow components"
  for app in ${PROW_COMPONENTS}; do
    do_kubectl wait pod \
      --for=condition=ready \
      --selector=app=${app} \
      --timeout=180s
  done
  echo "Prow components are ready"
}

function push_prow_job_image() {
  echo "Push test image to registry"
  docker pull gcr.io/k8s-prow/alpine
  docker tag gcr.io/k8s-prow/alpine:latest localhost:5000/alpine:latest
  docker push localhost:5000/alpine:latest
}

function main() {
  create_cluster_if_not_exist
  setup_cluster
  deploy_prow
  push_prow_job_image
}

main
