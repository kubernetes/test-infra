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

# script adapted from - https://gist.github.com/aojea/2c94034f8e86d08842e5916231eb3fe1

set -e
set -o pipefail

build_docker(){
  build/run.sh make all WHAT="cmd/kubectl cmd/kubelet" 1> /dev/null
  make quick-release-images 1> /dev/null
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

update_control_plane(){
  # TODO allow to configure node and control plane components
  for n in $CONTROL_PLANE_NODES; do
    for i in $CONTROL_PLANE_COMPONENTS; do
      kind load image-archive ${IMAGES_PATH}/${i}.tar --name ${CLUSTER_NAME} --nodes $n
      docker exec $n sed -e '/image:.*-amd64:/!s|\(image:.*\):.*|\1-amd64:'"${DOCKER_TAG}"'|' -i /etc/kubernetes/manifests/$i.yaml
      echo "Updated component $i on node $n"
      sleep 1
    done
  done
}

usage()
{
    echo "usage: kind_upgrade.sh [-n|--name <cluster_name>]"
    echo "                       [--no-control-plane]  [--no-kubelet]"
    echo ""
}

parse_args()
{
    while [ "$1" != "" ]; do
        case $1 in
            -n | --name )			shift
                                          	CLUSTER_NAME=$1
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

# Set default values
CLUSTER_NAME=${CLUSTER_NAME:-kind}
BUILD_MODE=${BUILD_MODE:-docker}
UPDATE_KUBE_PROXY=${UPDATE_KUBE_PROXY:-true}
UPDATE_KUBELET=${UPDATE_KUBE_PROXY:-true}
# TODO: we can have more granularity here
UPDATE_CONTROL_PLANE=${UPDATE_CONTROL_PLANE:-true}
CONTROL_PLANE_COMPONENTS="kube-apiserver kube-controller-manager kube-scheduler"

parse_args $*

# Initialize variables
# Assume go installed
KUBE_ROOT="."
# KUBE_ROOT="$(go env GOPATH)/src/k8s.io/kubernetes"
source "${KUBE_ROOT}/hack/lib/version.sh"
kube::version::get_version_vars
DOCKER_TAG=${KUBE_GIT_VERSION/+/_}
DOCKER_REGISTRY=${KUBE_DOCKER_REGISTRY:-registry.k8s.io}
export GOFLAGS="-tags=providerless"
export KUBE_BUILD_CONFORMANCE=n

# KIND nodes
NODES=$(kind get nodes --name ${CLUSTER_NAME})
CONTROL_PLANE_NODES=$(kind get nodes --name ${CLUSTER_NAME} | grep control)
WORKER_NODES=$(kind get nodes --name ${CLUSTER_NAME} | grep worker)

# Main
build_docker
IMAGES_PATH="${KUBE_ROOT}/_output/release-images/amd64"
KUBELET_BINARY=$(find ${KUBE_ROOT}/_output/ -type f -name kubelet)

if [[ "$UPDATE_CONTROL_PLANE" == "true" ]]; then
  update_control_plane
fi

if [[ "$UPDATE_KUBELET" == "true" ]]; then
  update_kubelet
fi

if kubectl get nodes | grep NoReady; then
  echo "Error: KIND cluster $CLUSTER_NAME NOT READY"
  exit 1
else
  echo "KIND cluster $CLUSTER_NAME updated successfully with version $KUBE_GIT_VERSION"
  exit 0
fi
