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

# cd to the repo root and setup go
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd "${REPO_ROOT}"
source hack/build/setup-go.sh

# build gotestsum
cd 'hack/tools'
go build -o "${REPO_ROOT}/_bin/gotestsum" gotest.tools/gotestsum
cd "${REPO_ROOT}"

JUNIT_RESULT_DIR="${REPO_ROOT}/_output"
# if we are in CI, copy to the artifact upload location
if [[ -n "${ARTIFACTS:-}" ]]; then
  JUNIT_RESULT_DIR="${ARTIFACTS}"
fi

# run unit tests with junit output
(
  set -x;
  mkdir -p "${JUNIT_RESULT_DIR}"
  "${REPO_ROOT}/_bin/gotestsum" --junitfile="${JUNIT_RESULT_DIR}/junit-unit.xml" \
    -- './...'
)
