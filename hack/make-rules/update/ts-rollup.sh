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

    # For development purpose, making sure that the rolled js files are
    # identical with prod
    if [[ "${1:-}" != "--verify" ]]; then
        continue
    fi
    if [[ "$rollup_entrypoint_dir" =~ gopherage ]]; then
        continue
    fi
    rollup_entrypoint_package_name="$(basename ${rollup_entrypoint_dir})"
    url="https://prow.k8s.io/static/${rollup_entrypoint_package_name/-/_/}_bundle.min.js"
    if [[ "$rollup_entrypoint_dir" =~ prow/spyglass/lenses ]]; then
        url="https://prow.k8s.io/spyglass/static/${rollup_entrypoint_package_name}/script_bundle.min.js"
    fi
    downloaded="${rollup_entrypoint_dir}/downloaded.${rollup_entrypoint_package_name}.bundle.min.js"
    curl $url -o $downloaded
    diff $downloaded $OUT || {
        echo "ERROR: not the same"
        rm $downloaded
        exit 1
    }
    rm $downloaded
done
