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

# build golangci-lint
echo "Install golangci-lint."
cd "hack/tools"
go build -o "${REPO_ROOT}/_bin/golangci-lint" github.com/golangci/golangci-lint/cmd/golangci-lint
cd "${REPO_ROOT}"

echo "Run golangci-lint."
echo "Go version: $(go version)"
echo "Golangci-lint version: $(./_bin/golangci-lint version)"
export GO111MODULE=on
export GOPROXY=https://proxy.golang.org
export GOSUMDB=sum.golang.org
./_bin/golangci-lint --config ".golangci.yml" run ./...
