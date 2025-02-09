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

set -e
set -o pipefail

build_docker(){
  build/run.sh make all WHAT="cmd/kubectl cmd/kubelet" 1> /dev/null
  make quick-release-images 1> /dev/null
}

build_bazel(){
  bazel build //cmd/kubectl:kubectl //cmd/kubelet:kubelet //build:docker-artifacts
}

update_kubelet() {
 for n in $NODES; do
   # Backup previous kubelet
   docker exec $n cp /usr/bin/kubelet /usr/bin/kubelet.bak
   # Install new kubelet binary
   docker cp ${KUBELET_BINARY} $n:/usr/bin/kubelet
   docker exec $n systemctl restart kubelet
  echo "Updated kubelet on node $n"
 done
}

update_kube_proxy() {
  for n in $NODES; do
   kind load image-archive ${IMAGES_PATH}/kube-proxy.tar --name ${CLUSTER_NAME}
  done
  # RollingUpdate
  kubectl set image ds/kube-proxy kube-proxy=${DOCKER_REGISTRY}/kube-proxy-amd64:${DOCKER_TAG} -n kube-system
  kubectl rollout status ds/kube-proxy -n kube-system -w
  echo "Updated kube-proxy"
}

update_cni() {
  for n in $NODES; do
   kind load image-archive ${CNI_IMAGE} --name ${CLUSTER_NAME}
  done
  # RollingUpdate
  kubectl set image ds/kindnet kindnet-cni=${CNI_IMAGE} -n kube-system
  kubectl rollout status ds/kindnet -n kube-system -w
  echo "Updated kindnet"
}

update_control_plane(){
  # TODO allow to configure node and control plane components
  for n in $CONTROL_PLANE_NODES; do
    for i in $CONTROL_PLANE_COMPONENTS; do
      kind load image-archive ${IMAGES_PATH}/${i}.tar --name ${CLUSTER_NAME} --nodes $n
      docker exec $n sed -i.bak -r "s|^(.*image\:.*)\:.*$|\1-amd64\:${DOCKER_TAG}|" /etc/kubernetes/manifests/$i.yaml
      echo "Updated component $i on node $n"
      sleep 1
    done
  done
}

usage()
{
    echo "usage: kind_upgrade.sh [-n|--name <cluster_name>] [--cni <cni_image>] [-b|--build-mode docker|bazel]"
    echo "                       [--no-kproxy]  [--no-control-plane]  [--no-kubelet]"
    echo ""
}

parse_args()
{
    while [ "$1" != "" ]; do
        case $1 in
            -n | --name )			shift
                                          	CLUSTER_NAME=$1
                                          	;;
            --cni-image )	           	shift
                                          	CNI_IMAGE=$1
                                          	;;
            -b | --build-mode )			shift
                                                if [ "$1" != "docker" ] && [ "$1" != "bazel" ]; then
                                                    echo "Invalid build mode: $1"
                                                    usage
                                                    exit 1
                                                fi
                                                BUILD_MODE=$1
                                                ;;
            --no-kproxy )                   	UPDATE_KUBE_PROXY=false
                                                ;;
            --no-kubelet )                   	UPDATE_KUBELET=false
                                                ;;
            --no-control-plane )               	UPDATE_CONTROL_PLANE=false
                                                ;;
            -h | --help )                       usage
                                                exit
                                                ;;
            * )                                 usage
                                                exit 1
        esac
        shift
    done
}

parse_args $*

# Set default values
CLUSTER_NAME=${CLUSTER_NAME:-kind}
BUILD_MODE=${BUILD_MODE:-docker}
UPDATE_KUBE_PROXY=${UPDATE_KUBE_PROXY:-true}
UPDATE_KUBELET=${UPDATE_KUBE_PROXY:-true}
# TODO: we can have more granularity here
UPDATE_CONTROL_PLANE=${UPDATE_CONTROL_PLANE:-true}
CONTROL_PLANE_COMPONENTS="kube-apiserver kube-controller-manager kube-scheduler"

# Initialize variables
# Assume go installed
KUBE_ROOT="."
# KUBE_ROOT="$(go env GOPATH)/src/k8s.io/kubernetes"
source "${KUBE_ROOT}/hack/lib/version.sh"
kube::version::get_version_vars
DOCKER_TAG=${KUBE_GIT_VERSION/+/_}
DOCKER_REGISTRY=${KUBE_DOCKER_REGISTRY:-k8s.gcr.io}
export GOFLAGS="-tags=providerless"
export KUBE_BUILD_CONFORMANCE=n

# KIND nodes
NODES=$(kind get nodes --name ${CLUSTER_NAME})
CONTROL_PLANE_NODES=$(kind get nodes --name ${CLUSTER_NAME} | grep control)
WORKER_NODES=$(kind get nodes --name ${CLUSTER_NAME} | grep worker)

# Main
if [[ "$BUILD_MODE" == "docker" ]]; then
  build_docker
  IMAGES_PATH="${KUBE_ROOT}/_output/release-images/amd64"
  KUBELET_BINARY=$(find ${KUBE_ROOT}/_output/ -type f -name kubelet)
else
  build_bazel
  IMAGES_PATH="${KUBE_ROOT}/bazel-kubernetes/bazel-out/k8-fastbuild/bin/build"
  KUBELET_BINARY=$(find ${KUBE_ROOT}/bazel-kubernetes/ -type f -name kubelet)
fi

if [[ "$UPDATE_CONTROL_PLANE" == "true" ]]; then
  update_control_plane
fi

if [[ "$UPDATE_KUBELET" == "true" ]]; then
  update_kubelet
fi

if [[ "$UPDATE_KUBE_PROXY" == "true" ]]; then
  update_kube_proxy
fi

# If CNI_IMAGE set update the CNI  
if [[ ! -z ${CNI_IMAGE} ]]; then
  update_cni
fi

if kubectl get nodes | grep NoReady; then
  echo "Error: KIND cluster $CLUSTER_NAME NOT READY"
  exit 1
else
  echo "KIND cluster $CLUSTER_NAME updated successfully with version $KUBE_GIT_VERSION"
  exit 0
fi