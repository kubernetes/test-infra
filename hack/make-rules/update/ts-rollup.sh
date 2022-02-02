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

set -o nounset
set -o errexit
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd $REPO_ROOT

readonly TS_PACKAGES_FILE=".ts-packages"
ROLLUP_ENTRYPOINTS=()
while IFS= read -r rollup_entrypoint
do
    ROLLUP_ENTRYPOINTS+=("${rollup_entrypoint}")
done < "${TS_PACKAGES_FILE}"

for rollup_entrypoint in ${ROLLUP_ENTRYPOINTS[@]}; do
    if [[ -z  "${rollup_entrypoint}" || ${rollup_entrypoint} =~ \#.* ]]; then
        continue
    fi
    echo "Rollup ${rollup_entrypoint}"
    rollup_entrypoint_dir="$(dirname ${rollup_entrypoint})"
    rollup_entrypoint_file="$(basename -s '.ts' ${rollup_entrypoint})"
    export OUT="${rollup_entrypoint_dir}/zz.${rollup_entrypoint_file}.bundle.min.js"
    ./hack/rollup-js.sh "${rollup_entrypoint_dir}" "${rollup_entrypoint_file}"
done
