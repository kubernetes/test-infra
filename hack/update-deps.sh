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


# Update vendor and bazel rules to match go.mod
#
# Usage:
#   update-deps.sh [--patch|--minor] [packages]

set -o nounset
set -o errexit
set -o pipefail

if [[ -n "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then # Running inside bazel
  echo "Updating modules..." >&2
elif ! command -v bazel &>/dev/null; then
  echo "Install bazel at https://bazel.build" >&2
  exit 1
elif ! bazel query @io_k8s_test_infra//vendor/github.com/bazelbuild/bazel-gazelle/cmd/gazelle &>/dev/null; then
  (
    set -o xtrace
    bazel run @io_k8s_test_infra//hack:bootstrap-testinfra
    bazel run @io_k8s_test_infra//hack:update-bazel
  )
  exit 0
else
  (
    set -o xtrace
    bazel run @io_k8s_test_infra//hack:update-deps -- "$@"
  )
  exit 0
fi

go=$(realpath "$1")
export PATH=$(dirname "$go"):$PATH
gazelle=$(realpath "$2")
kazel=$(realpath "$3")
update_bazel=(
  $(realpath "$4")
  "$gazelle"
  "$kazel"
)

shift 4

cd "$BUILD_WORKSPACE_DIRECTORY"
trap 'echo "FAILED" >&2' ERR

prune-vendor() {
  find vendor -type f \
    -not -iname "*.c" \
    -not -iname "*.go" \
    -not -iname "*.h" \
    -not -iname "*.proto" \
    -not -iname "*.s" \
    -not -iname "AUTHORS*" \
    -not -iname "CONTRIBUTORS*" \
    -not -iname "COPYING*" \
    -not -iname "LICENSE*" \
    -not -iname "NOTICE*" \
    -delete
}

export GO111MODULE=on
mode="${1:-}"
shift || true
case "$mode" in
--minor)
    "$go" get -u "$@"
    ;;
--patch)
    "$go" get -u=patch "$@"
    ;;
"")
    # Just validate, or maybe manual go.mod edit
    ;;
*)
    echo "Usage: $(basename "$0") [--patch|--minor] [packages]" >&2
    exit 1
    ;;
esac

rm -rf vendor
export GOPROXY=https://proxy.golang.org
export GOSUMDB=sum.golang.org
"$go" mod tidy
"$go" mod vendor
prune-vendor
touch ./vendor/BUILD.bazel
"$gazelle" update-repos --from_file=go.mod --to_macro=repos.bzl%go_repositories
"${update_bazel[@]}"
"${update_bazel[@]}"
echo "SUCCESS: updated modules"
