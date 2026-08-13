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

DOCKER_TAG=${LATEST_VERSION/+/_}
DOCKER_REGISTRY=${KUBE_DOCKER_REGISTRY:-registry.k8s.io}
export GOFLAGS="-tags=providerless"
export KUBE_BUILD_CONFORMANCE=n

# KIND nodes
NODES=$(kind get nodes --name ${CLUSTER_NAME})
CONTROL_PLANE_NODES=$(kind get nodes --name ${CLUSTER_NAME} | grep control)
WORKER_NODES=$(kind get nodes --name ${CLUSTER_NAME} | grep worker)

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

download_images(){
  curl -L 'https://dl.k8s.io/ci/'${LATEST_VERSION}'/kubernetes-server-linux-amd64.tar.gz' > kubernetes-server-linux-amd64.tar.gz
  tar -xf kubernetes-server-linux-amd64.tar.gz
}

update_kubelet() {
 for n in $NODES; do
   # Backup previous kubelet
   docker exec $n cp /usr/bin/kubelet /usr/bin/kubelet.bak
   # This flag has been removed in 1.35. We can remove the replacement once 1.35 cannot be emulated anymore
   docker exec $n sed -i 's/--pod-infra-container-image=[^ "]*//g' /var/lib/kubelet/kubeadm-flags.env
   # Install new kubelet binary
   docker cp ${KUBELET_BINARY} $n:/usr/bin/kubelet
   docker exec $n systemctl restart kubelet
  echo "Updated kubelet on node $n"
 done
}

update_control_plane(){
  local emulation_forward_compatible=$1
  # TODO allow to configure node and control plane components
  for n in $CONTROL_PLANE_NODES; do
    for i in $CONTROL_PLANE_COMPONENTS; do
      kind load image-archive ${IMAGES_PATH}/${i}.tar --name ${CLUSTER_NAME} --nodes $n
      if [ "$emulation_forward_compatible" == "true" ]; then
        # set --emulation-forward-compatible=true for apiserver if not already set
        docker exec $n bash -c "grep -q 'emulation-forward-compatible=true' /etc/kubernetes/manifests/$i.yaml || sed -e '/- kube-apiserver/a \ \ \ \ - --emulation-forward-compatible=true' -i /etc/kubernetes/manifests/$i.yaml"
      else
        # remove --emulation-forward-compatible=true
        docker exec $n sed -e '/- --emulation-forward-compatible=true/d' -i /etc/kubernetes/manifests/$i.yaml
      fi
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

# Main
TMP_DIR=$(mktemp -d -p /tmp kind-e2e-XXXXXX)
echo "Created temporary directory: ${TMP_DIR}"
pushd $TMP_DIR
download_images
popd
IMAGES_PATH="${TMP_DIR}/kubernetes/server/bin"
KUBELET_BINARY=$(find ${TMP_DIR}/kubernetes/server/bin/ -type f -name kubelet)

if [[ "$UPDATE_CONTROL_PLANE" == "true" ]]; then
  update_control_plane "true"
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
