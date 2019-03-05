#!/usr/bin/env bash
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
set -o xtrace

source $(dirname "${BASH_SOURCE[0]}")/kind-e2e.sh

run_tests() {
  # binaries needed by the conformance image
  rm -rf _output/bin
  make WHAT="test/e2e/e2e.test vendor/github.com/onsi/ginkgo/ginkgo cmd/kubectl"

  # grab the version number for kubernetes
  export KUBE_ROOT=${PWD}
  source ${KUBE_ROOT}/hack/lib/version.sh
  kube::version::get_version_vars

  export VERSION=$(echo -n "${KUBE_GIT_VERSION}" | cut -f 1 -d '+')
  export KUBECONFIG=$(kind get kubeconfig-path)

  pushd ${PWD}/cluster/images/conformance

  # build and load the conformance image into the kind nodes
  make build ARCH=amd64
  kind load docker-image k8s.gcr.io/conformance-amd64:${VERSION}

  # patch the image in manifest
  sed -i "s|conformance-amd64:.*|conformance-amd64:${VERSION}|g" conformance-e2e.yaml
  ./conformance-e2e.sh

  popd

  # extract the test results
  NODE_NAME=$(kubectl get po -n conformance e2e-conformance-test -o 'jsonpath={.spec.nodeName}')
  docker exec ${NODE_NAME} tar cvf - /tmp/results | tar -C ${ARTIFACTS} --strip-components 2 -xf -
}

main
