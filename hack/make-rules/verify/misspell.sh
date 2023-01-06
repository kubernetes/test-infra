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

BIN_DIR="${REPO_ROOT}/_bin/misspell"
mkdir -p "$BIN_DIR"
MISSPELL="${BIN_DIR}/misspell"

echo "Ensuring go version."
source ./hack/build/setup-go.sh

# build misspell
echo "Install misspell."
cd "hack/tools"
go build -o "$MISSPELL" github.com/client9/misspell/cmd/misspell
cd "${REPO_ROOT}"

trap 'echo ERROR: found unexpected instance of "Git"hub, use github or GitHub' ERR

echo "Check for word 'github'..."
# Unit test: Git"hub (remove ")
# Appear to need to use this if statement on mac to get the not grep to work
if find -L . -type f -not \( \
  \( \
    -path '*/vendor/*' \
    -o -path '*/external/*' \
    -o -path '*/static/*' \
    -o -path '*/third_party/*' \
    -o -path '*/node_modules/*' \
    -o -path '*/localdata/*' \
    -o -path '*/gubernator/*' \
    -o -path '*/prow/bugzilla/client_test.go' \
    -o -path './.git/*' \
    -o -path './_bin/*' \
    -o -path './_output/*' \
    -o -path './_artifacts/*' \
    -o -path './bazel-*/*' \
    -o -path './.python_virtual_env/*' \
    \) -prune \
    \) -exec grep -Hn 'Git'hub '{}' '+' ; then
  echo "Failed"
  false
fi


trap 'echo ERROR: bad spelling, fix with hack/update-spelling.sh' ERR

echo "Check for spelling..."
# Unit test: lang auge (remove space)
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
    \) -exec "$MISSPELL" '{}' '+'

echo 'PASS: No spelling issues detected'
