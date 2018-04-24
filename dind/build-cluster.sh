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

# Create a tmpdir for the Docker build's context.
CLUSTER_DIR=$(mktemp -d)
echo "Building cluster docker image from ${CLUSTER_DIR}"


cd ${KUBE_ROOT}

# The outer layer doesn't run node or master components. But tests need kubectl.
bazel build //build/debs:kubectl --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64

# Tests are run from the outer layer, so package the test artifacts.
bazel build //vendor/github.com/onsi/ginkgo/ginkgo:ginkgo --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64
bazel build //test/e2e:e2e.test --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64

cd -

cd ${TOOL_ROOT}
bazel build //dind/cmd/cluster-up:cluster-up --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64
cd -

# Copy artifacts into the tmpdir for Docker's context.

cp ${KUBE_ROOT}/bazel-bin/build/debs/kubectl.deb ${CLUSTER_DIR}

# Tests are only run against one platform (linux/amd64), so no searching logic.
cp ${KUBE_ROOT}/bazel-bin/vendor/github.com/onsi/ginkgo/ginkgo/linux_amd64_stripped/ginkgo ${CLUSTER_DIR}
cp ${KUBE_ROOT}/bazel-bin/test/e2e/e2e.test ${CLUSTER_DIR}

# Get version info in a file. Kubeadm version and docker tags might vary slightly.
cat ${KUBE_ROOT}/bazel-out/stable-status.txt | grep STABLE_BUILD_SCM_REVISION | awk '{print $2}' > ${CLUSTER_DIR}/source_version
cat ${KUBE_ROOT}/bazel-out/stable-status.txt | grep STABLE_DOCKER_TAG | awk '{print $2}' > ${CLUSTER_DIR}/docker_version

# Get systemd wrapper (needed until we put cluster-up into a systemd target).
cp ${TOOL_ROOT}/init-wrapper.sh ${CLUSTER_DIR}
# Get the cluster-up script.
cp ${TOOL_ROOT}/start.sh ${CLUSTER_DIR}
# Get the test execution script.
cp ${TOOL_ROOT}/dind-test.sh ${CLUSTER_DIR}

cp ${TOOL_ROOT}/../bazel-bin/dind/cmd/cluster-up/linux_amd64_pure_stripped/cluster-up ${CLUSTER_DIR}

cp ${TOOL_ROOT}/cluster/Dockerfile ${CLUSTER_DIR}/Dockerfile
cp ${TOOL_ROOT}/cluster/Makefile ${CLUSTER_DIR}/Makefile

# Create the dind-cluster container
cd ${CLUSTER_DIR}
docker save k8s.gcr.io/dind-node-amd64:$(cat docker_version) -o dind-node-bundle.tar
make build K8S_VERSION=$(cat docker_version)
cd -

rm -rf ${CLUSTER_DIR}
