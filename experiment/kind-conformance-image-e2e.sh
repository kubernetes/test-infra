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

# our exit handler (trap)
cleanup() {
    # always attempt to dump logs
    kind "export" logs "${ARTIFACTS}/logs" || true
    # KIND_IS_UP is true once we: kind create
    if [[ "${KIND_IS_UP:-}" = true ]]; then
        kind delete cluster || true
    fi
    # clean up e2e.test symlink
    rm -f _output/bin/e2e.test
    # remove our tempdir
    # NOTE: this needs to be last, or it will prevent kind delete
    if [[ -n "${TMP_DIR:-}" ]]; then
        rm -rf "${TMP_DIR}"
    fi
}

# install kind to a tempdir GOPATH from this script's kind checkout
install_kind() {
    # install `kind` to tempdir
    TMP_DIR=$(mktemp -d)
    curl -sLo "${TMP_DIR}/kind" https://kind.sigs.k8s.io/dl/latest/kind-linux-amd64
    chmod +x "${TMP_DIR}/kind"
    PATH="${TMP_DIR}:${PATH}"
    export PATH
}

# build kubernetes / node image, e2e binaries
build() {
    # possibly enable bazel build caching before building kubernetes
    BAZEL_REMOTE_CACHE_ENABLED=${BAZEL_REMOTE_CACHE_ENABLED:-false}
    if [[ "${BAZEL_REMOTE_CACHE_ENABLED}" == "true" ]]; then
        # run the script in the kubekins image, do not fail if it fails
        /usr/local/bin/create_bazel_cache_rcs.sh || true
    fi

    # build the node image w/ kubernetes
    kind build node-image --type=bazel --kube-root="${PWD}"

    # try to make sure the kubectl we built is in PATH
    local maybe_kubectl
    maybe_kubectl="$(bazel aquery 'mnemonic("GoLink", //cmd/kubectl)' \
      | grep '^  Outputs: \[.*\]$' \
      | sed 's/^  Outputs: \[\(.*\)\]$/\1/')"
    if [[ -n "${maybe_kubectl}" ]]; then
        PATH="$(dirname "${maybe_kubectl}"):${PATH}"
        export PATH
    fi

    # release some memory after building
    sync || true
    echo 1 > /proc/sys/vm/drop_caches || true
}


# up a cluster with kind
create_cluster() {
    # create the config file
    cat <<EOF > "${ARTIFACTS}/kind-config.yaml"
# config for 1 control plane node and 2 workers
# necessary for conformance
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
# the control plane node
- role: control-plane
- role: worker
- role: worker
EOF
    # mark the cluster as up for cleanup
    # even if kind create fails, kind delete can clean up after it
    KIND_IS_UP=true

    KUBECONFIG="${HOME}/.kube/kind-config-default"
    export KUBECONFIG

    # actually create, with:
    # - do not delete created nodes from a failed cluster create (for debugging)
    # - wait up to one minute for the nodes to be "READY"
    # - set log leve to debug
    # - use our multi node config
    kind create cluster \
        --image=kindest/node:latest \
        --retain \
        --wait=1m \
        --loglevel=debug \
        "--config=${ARTIFACTS}/kind-config.yaml"
}


run_tests() {
  # binaries needed by the conformance image
  rm -rf _output/bin
  NEW_GO_RUNNER_DIR="cluster/images/conformance/go-runner"
  if [ -d "$NEW_GO_RUNNER_DIR" ]; then
      make WHAT="test/e2e/e2e.test vendor/github.com/onsi/ginkgo/ginkgo cmd/kubectl cluster/images/conformance/go-runner"
  else
      make WHAT="test/e2e/e2e.test vendor/github.com/onsi/ginkgo/ginkgo cmd/kubectl"
  fi

  # grab the version number for kubernetes
  export KUBE_ROOT=${PWD}
  source "${KUBE_ROOT}/hack/lib/version.sh"
  kube::version::get_version_vars

  VERSION=$(echo -n "${KUBE_GIT_VERSION}" | cut -f 1 -d '+')
  export VERSION

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
  docker exec "${NODE_NAME}" tar cvf - /tmp/results | tar -C "${ARTIFACTS}" --strip-components 2 -xf -
}

# setup kind, build kubernetes, create a cluster, run the e2es
main() {
    # ensure artifacts exists when not in CI
    ARTIFACTS="${ARTIFACTS:-${PWD}/_artifacts}"
    mkdir -p "${ARTIFACTS}"
    export ARTIFACTS
    # now build an run the cluster and tests
    trap cleanup EXIT
    install_kind
    build
    create_cluster
    run_tests
}

main
