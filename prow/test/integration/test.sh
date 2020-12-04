#!/usr/bin/env bash
set -o errexit

CURRENT_REPO="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd -P )"

if [[ "$1" == "--create-cluster" ]]; then
  "${CURRENT_REPO}/setup-cluster.sh"
fi

"${CURRENT_REPO}/setup-prow.sh"

bazel test //prow/test/integration/test:go_default_test
