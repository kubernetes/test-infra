#!/usr/bin/env bash
# Copyright 2022 The Kubernetes Authors.
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

# Auto-detect architecture
if [[ "${GOARCH:-}" == "" ]]; then
  HOST_ARCH=$(go env GOARCH)
else
  HOST_ARCH="${GOARCH}"
fi

# Since the compiled tools are mounted into a Linux container, it's more important to get the ARCH right
# defaulting to Linux to avoids dynamically building for Darwin which breaks exec'ing in the container at later stages
TARGET_OS="linux"
TARGET_ARCH=${TARGET_ARCH:-$HOST_ARCH}

echo "Building tools for OS: ${TARGET_OS}, Architecture: ${TARGET_ARCH}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${REPO_ROOT}"
source hack/build/setup-go.sh

BIN_DIR="_bin"
CMD_ARGS=""

for tool in config-rotator config-forker generate-tests; do
    GOOS=${TARGET_OS} GOARCH=${TARGET_ARCH} go build -o "${BIN_DIR}/${tool}" "${REPO_ROOT}/releng/${tool}"
    CMD_ARGS+="${BIN_DIR}/${tool} "
done

hack/run-in-python-container.sh releng/prepare_release_branch.py $CMD_ARGS
