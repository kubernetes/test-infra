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

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${REPO_ROOT}"
source hack/build/setup-go.sh

BIN_DIR="_bin"
CMD_ARGS=""

for tool in config-rotator config-forker; do
    GOOS=linux GOARCH=amd64 go build -o "${BIN_DIR}/${tool}" "${REPO_ROOT}/releng/${tool}"
    CMD_ARGS+="${BIN_DIR}/${tool} "
done

CMD_ARGS+="releng/generate_tests.py"
hack/run-in-python-container.sh releng/prepare_release_branch.py $CMD_ARGS
