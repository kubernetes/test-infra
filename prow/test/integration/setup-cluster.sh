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

# create registry container unless it already exists
reg_name='kind-registry'
reg_port='5000'
running="$(docker inspect -f '{{.State.Running}}' "${reg_name}" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  docker run \
    -d --restart=always -p "${reg_port}:5000" --name "${reg_name}" \
    registry:2
fi

# create a cluster with the local registry enabled in containerd,
# as well as configure node-lables and extraPortMappings for ingress,
# see: https://kind.sigs.k8s.io/docs/user/ingress/#create-cluster
cluster_name='kind-prow-integration'
running="$(docker inspect -f '{{.State.Running}}' "${cluster_name}-control-plane" 2>/dev/null || true)"
if [ "${running}" != 'true' ]; then
  echo "Create kind cluster"
cat <<EOF | kind create cluster --name ${cluster_name} --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localhost:${reg_port}"]
    endpoint = ["http://${reg_name}:${reg_port}"]
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

# connect the registry to the cluster network
# (the network may already be connected)
echo "Set up local registry for cluster"
docker network connect "kind" "${reg_name}" || true

# Ensure working on kind cluster
CONTEXT="$(kubectl config current-context)"
if [[ -z "${CONTEXT}" ]]; then
  echo "Current kube context cannot be empty"
  exit 1
fi
if [[ "${CONTEXT}" != "kind-kind-prow-integration" ]]; then
  echo "Current kube context is '${CONTEXT}', has to be kind-kind-prow-integration"
  exit 1
fi

# Document the local registry
# https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${reg_port}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF

# Install nginx
echo "Install nginx on kind cluster"
# Pin the ingress-nginx manifest to 9bf4155724d8396a70129c8d06eb970d79795d92 on 01/13/2021.
kubectl --context=${CONTEXT} apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/9bf4155724d8396a70129c8d06eb970d79795d92/deploy/static/provider/kind/deploy.yaml

exit 0
