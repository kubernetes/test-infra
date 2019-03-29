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


# Run dep ensure and generate bazel rules.
#
# Usage:
#   update-deps.sh <ARGS>
#
# The args are sent to dep ensure -v <ARGS>

set -o nounset
set -o errexit
set -o pipefail

cd "$(git rev-parse --show-toplevel)"
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

mode="${1:-}"
shift || true
case "$mode" in
--minor)
    bazel run //:update-minor -- "$@"
    ;;
--patch)
    bazel run //:update-patch -- "$@"
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
export GO111MODULE=on
bazel run //:go -- mod tidy
bazel run //:go -- mod vendor
prune-vendor
hack/update-bazel.sh
bazel run //:gazelle -- update-repos --from_file=go.mod
hack/update-bazel.sh # TODO(fejta): seems to cause weird changes otherwise
echo SUCCESS
