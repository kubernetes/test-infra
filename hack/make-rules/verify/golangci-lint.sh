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

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd $REPO_ROOT

trap 'echo ERROR: golangci-lint failed >&2' ERR

echo "Ensuring go version."
source ./hack/build/setup-go.sh

if [[ ! -f .golangci.yml ]]; then
  echo 'ERROR: missing .golangci.yml in repo root' >&2
  exit 1
fi

echo "Run golangci-lint."
echo "Golangci-lint version: $(go tool -modfile="${REPO_ROOT}/hack/tools/golangci-lint/go.mod" golangci-lint version)"
export GO111MODULE=on
export GOPROXY=https://proxy.golang.org
export GOSUMDB=sum.golang.org
go tool -modfile="${REPO_ROOT}/hack/tools/golangci-lint/go.mod" golangci-lint \
  --config ".golangci.yml" run ./...
