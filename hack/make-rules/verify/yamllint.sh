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

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd "${REPO_ROOT}"

DOCKER=(docker)

if [[ -n "${NO_DOCKER:-}" ]]; then
  DOCKER=(echo docker)
elif ! (command -v docker >/dev/null); then
  echo "WARNING: docker not installed; please install docker or try setting NO_DOCKER=true" >&2
  exit 1
fi

LINT_COMMAND=("yamllint" "-c" "config/jobs/.yamllint.conf" "config/jobs" "config/prow/cluster")

"${DOCKER[@]}" run \
    --rm -i \
    -v "${REPO_ROOT:?}:${REPO_ROOT:?}" -w "${REPO_ROOT}" \
    --security-opt="label=disable" \
    "quay.io/kubermatic/yamllint:0.1" \
    "${LINT_COMMAND[@]}"

if [[ -n "${NO_DOCKER:-}" ]]; then
  (
    set -o xtrace
    "${LINT_COMMAND[@]}"
  )
fi
