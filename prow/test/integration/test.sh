#!/usr/bin/env bash
set -o errexit

CURRENT_REPO="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd -P )"

if [[ -n "${CI:-}" ]]; then
  # TODO(chaodaiG): remove this once kind is installed in test image
  echo "Install KIND in prow"
  curl -Lo /usr/bin/kind https://kind.sigs.k8s.io/dl/v0.9.0/kind-linux-amd64
  chmod +x /usr/bin/kind

  # TODO(chaodaiG): remove this once bazel is installed in test image
  mkdir -p "/usr/local/lib/bazel/bin"
  pushd "/usr/local/lib/bazel/bin" >/dev/null
  curl -LO https://releases.bazel.build/3.0.0/release/bazel-3.0.0-linux-x86_64
  chmod +x bazel-3.0.0-linux-x86_64
  popd
fi

if [[ "$1" == "--create-cluster" ]]; then
  "${CURRENT_REPO}/setup-cluster.sh"
fi

"${CURRENT_REPO}/setup-prow.sh"

bazel test //prow/test/integration/test:go_default_test
