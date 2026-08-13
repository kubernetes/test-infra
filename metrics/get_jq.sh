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

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${REPO_ROOT}"

JQ_DIR="_bin/jq-1.5"
JQ_BIN="${REPO_ROOT}/${JQ_DIR}/jq"
if [[ -f "${JQ_BIN}" ]]; then
    echo "$JQ_BIN"
    exit 0
fi

bin-path() {
    echo "${REPO_ROOT}/${JQ_DIR}/$1"
}

download() {
    local name="$1"
    local path="$(bin-path $name)"
    curl -fsSL "https://github.com/stedolan/jq/releases/download/jq-1.5/$name" -o "$path"
    chmod a+x "$path"
    cp "$path" "$JQ_BIN"
}

mkdir -p "${JQ_DIR}"
# linux64 is used by CI, making sure that this is used in CI as well
download jq-linux64
# ensure that `_bin/jq-1.5/jq` is compatible with host, so that python3 test
# won't fail locally
if [[ "$(uname)" == Darwin ]]; then
    download jq-osx-amd64
fi

echo "$JQ_BIN"
