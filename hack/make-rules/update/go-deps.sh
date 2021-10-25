#!/usr/bin/env bash
# Copyright 2021 The Kubernetes Authors.
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

set -o nounset
set -o errexit
set -o pipefail

if [[ -z "${REPO_ROOT:-}" ]]; then
  REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
fi
cd $REPO_ROOT

echo "Ensuring go version."
source ./hack/build/setup-go.sh

trap 'echo "FAILED" >&2' ERR

export GO111MODULE=on
export GOPROXY=https://proxy.golang.org
export GOSUMDB=sum.golang.org
mode="${1:-}"
shift || true
case "$mode" in
--minor)
    if [[ -z "$@" ]]; then
      go get -u ./...
    else
      go get -u "$@"
    fi
    ;;
--patch)
    if [[ -z "$@" ]]; then
      go get -u=patch ./...
    else
      go get -u=patch "$@"
    fi
    ;;
"")
    # Just validate, or maybe manual go.mod edit
    ;;
*)
    echo "Usage: $(basename "$0") [--patch|--minor] [packages]" >&2
    exit 1
    ;;
esac

echo "Updating go mod tidy"
echo "Go version: $(go version)"
go mod tidy
cd "${REPO_ROOT}/hack/tools"
go mod tidy
echo "SUCCESS: updated modules"
