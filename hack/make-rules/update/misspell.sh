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

echo "Ensuring go version."
source ./hack/build/setup-go.sh

# build misspell
echo "Install misspell."
cd "hack/tools"
go build -o "${REPO_ROOT}/_bin/misspell" github.com/client9/misspell/cmd/misspell
MISSPELL="${REPO_ROOT}/_bin/misspell/misspell"
cd "${REPO_ROOT}"

find -L . -type f -not \( \
  \( \
    -path '*/vendor/*' \
    -o -path '*/external/*' \
    -o -path '*/static/*' \
    -o -path '*/third_party/*' \
    -o -path '*/node_modules/*' \
    -o -path '*/localdata/*' \
    -o -path './.git/*' \
    -o -path './_bin/*' \
    -o -path './_output/*' \
    -o -path './_artifacts/*' \
    -o -path './bazel-*/*' \
    -o -path './hack/tools/go.mod' \
    -o -path './hack/tools/go.sum' \
    -o -path './.python_virtual_env/*' \
    \) -prune \
    \) | xargs "${MISSPELL}" -w
