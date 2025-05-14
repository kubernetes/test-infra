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

update_kubelet() {
 # restart kubelet in control plane node
 for n in $CONTROL_PLANE_NODES; do
   # Backup previous kubelet
   docker exec $n cp /usr/bin/kubelet /usr/bin/kubelet.bak
   # Install new kubelet binary
   docker cp ${KUBELET_BINARY} $n:/usr/bin/kubelet
   docker exec $n systemctl restart kubelet
   echo "Updated kubelet on node $n"
   sleep 10
 done
}

upgrade_emulated_version(){
  for n in $CONTROL_PLANE_NODES; do
    for i in $CONTROL_PLANE_COMPONENTS; do
      docker exec $n sed -e '/\s*--emulated-version=.*/d' -i /etc/kubernetes/manifests/$i.yaml
      echo "Removed emulated-version flag of component $i static pod on node $n if present"
      sleep 1
    done
  done
}

check_emulated_version_removed(){
  for n in $CONTROL_PLANE_NODES; do
    for i in $CONTROL_PLANE_COMPONENTS; do
      if docker exec $n ps aux | grep $i | grep emulated-version; then
        echo "Error: Component $i still have emulated-version flag"
        exit 1
      fi 
      echo "Confirm component $i does not have --emulated-version flag"
    done
  done
}

usage()
{
    echo "usage: emulated-version-upgrade.sh [-n|--name <cluster_name>]"
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

parse_args $*

# Set default values
CLUSTER_NAME=${CLUSTER_NAME:-kind}
UPDATE_KUBE_PROXY=${UPDATE_KUBE_PROXY:-true}
UPDATE_KUBELET=${UPDATE_KUBE_PROXY:-true}

# Set control plane components
UPDATE_CONTROL_PLANE=${UPDATE_CONTROL_PLANE:-true}
CONTROL_PLANE_COMPONENTS="kube-apiserver kube-controller-manager kube-scheduler"

# Initialize variables
KUBE_ROOT="."
source "${KUBE_ROOT}/hack/lib/version.sh"
kube::version::get_version_vars
DOCKER_TAG=${KUBE_GIT_VERSION/+/_}
DOCKER_REGISTRY=${KUBE_DOCKER_REGISTRY:-registry.k8s.io}

# KIND nodes
NODES=$(kind get nodes --name ${CLUSTER_NAME})
CONTROL_PLANE_NODES=$(kind get nodes --name ${CLUSTER_NAME} | grep control)

# Main
KUBELET_BINARY=$(find ${KUBE_ROOT}/_output/ -type f -name kubelet)

if [[ "$UPDATE_CONTROL_PLANE" == "true" ]]; then
  upgrade_emulated_version
fi

if [[ "$UPDATE_KUBELET" == "true" ]]; then
  update_kubelet
fi

if [[ "$UPDATE_CONTROL_PLANE" == "true" ]]; then
  check_emulated_version_removed
  exit 0
fi
