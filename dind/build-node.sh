#!/bin/bash

# Copyright 2018 The Kubernetes Authors.
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

TOOL_ROOT=${TOOL_ROOT:-"$(pwd)"}
KUBE_ROOT=${KUBE_ROOT:-"../../kubernetes"}
echo $TOOL_ROOT
echo $KUBE_ROOT

# Create a tmpdir to build the docker images from.
NODE_DIR=$(mktemp -d)
echo "Building node docker image from ${NODE_DIR}"

# make bazel-release doesn't produce everything we need, and it does a lot we don't need.
cd ${KUBE_ROOT}

# Build the debs
bazel build //build/debs:debs --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64

# Build the docker containers
bazel build //build:docker-artifacts --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64

# Build the addons configs.
bazel build //cluster/addons:addon-srcs --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64

cd -

# Get debs needed for kubernetes node. Docker's build context rejects symlinks.
cp ${KUBE_ROOT}/bazel-bin/build/debs/kubernetes-cni.deb ${NODE_DIR}
cp ${KUBE_ROOT}/bazel-bin/build/debs/kubelet.deb ${NODE_DIR}
cp ${KUBE_ROOT}/bazel-bin/build/debs/kubeadm.deb ${NODE_DIR}
cp ${KUBE_ROOT}/bazel-bin/build/debs/cri-tools.deb ${NODE_DIR}
cp ${KUBE_ROOT}/bazel-bin/build/debs/kubectl.deb ${NODE_DIR}

# Get the docker images for components. Docker's build context rejects symlinks.
cp ${KUBE_ROOT}/bazel-bin/build/kube-proxy.tar ${NODE_DIR}
cp ${KUBE_ROOT}/bazel-bin/build/kube-controller-manager.tar ${NODE_DIR}
cp ${KUBE_ROOT}/bazel-bin/build/kube-scheduler.tar ${NODE_DIR}
cp ${KUBE_ROOT}/bazel-bin/build//kube-apiserver.tar ${NODE_DIR}

# Get version info in a file. Kubeadm version and docker tags might vary slightly.
cat ${KUBE_ROOT}/bazel-out/stable-status.txt | grep STABLE_BUILD_SCM_REVISION | awk '{print $2}' > ${NODE_DIR}/source_version
cat ${KUBE_ROOT}/bazel-out/stable-status.txt | grep STABLE_DOCKER_TAG | awk '{print $2}' > ${NODE_DIR}/docker_version

# Get the metrics-server addon config. This is needed for HPA tests.
mkdir -p ${NODE_DIR}/cluster/addons/metrics-server/
cp ${KUBE_ROOT}/bazel-kubernetes/cluster/addons/metrics-server/* ${NODE_DIR}/cluster/addons/metrics-server/
rm ${NODE_DIR}/cluster/addons/metrics-server/OWNERS

# Get the startup scripts.
cp ${TOOL_ROOT}/init-wrapper.sh ${NODE_DIR}
cp ${TOOL_ROOT}/start.sh ${NODE_DIR}
cp ${TOOL_ROOT}/node/Dockerfile ${NODE_DIR}/Dockerfile
cp ${TOOL_ROOT}/node/Makefile ${NODE_DIR}/Makefile

# Create the dind-node container
cd ${NODE_DIR}
make build K8S_VERSION=$(cat docker_version)
cd -

rm -rf ${NODE_DIR}
