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

readonly ROLLUP_CONFIG="make-rollup.config.js"

ROLLUP_ENTRYPOINT_DIR="${1:-}"
if [[ -z $ROLLUP_ENTRYPOINT_DIR ]]; then
    echo "ERROR: rollup entrypoint dir must be provided."
    exit 1
fi

ROLLUP_ENTRYPOINT_FILE="${2:-}"
if [[ -z $ROLLUP_ENTRYPOINT_FILE ]]; then
    echo "ERROR: rollup entrypoint file must be provided."
    exit 1
fi

readonly JS_OUTPUT_DIR="_output/js"
mkdir -p "${JS_OUTPUT_DIR}"

./node_modules/.bin/rimraf dist

echo "Running tsc"
TSCONFIG="${ROLLUP_ENTRYPOINT_DIR}/tsconfig.json"
# tsc by default output to `<outDIR>/<rel-path>/<basename>.js`, for example for
# `prow/cmd/deck/static/job-history` dir, the out file is
# `_output/js/prow/cmd/deck/static/job-history/job-history.js`
ENTRYPOINT_BASENAME="$(basename $ROLLUP_ENTRYPOINT_DIR)"
JS_OUTPUT_FILE="${JS_OUTPUT_DIR}/${ROLLUP_ENTRYPOINT_DIR}/${ROLLUP_ENTRYPOINT_FILE}.js"
./node_modules/.bin/tsc -p "${TSCONFIG}" --outDir "${JS_OUTPUT_DIR}"

echo "Running rollup"
BUNDLE_OUTPUT_DIR="${JS_OUTPUT_DIR}/${ROLLUP_ENTRYPOINT_DIR}"
ROLLUP_OUT_FILE="${BUNDLE_OUTPUT_DIR}/bundle.js"
./node_modules/.bin/rollup --environment "ROLLUP_OUT_FILE:${ROLLUP_OUT_FILE},ROLLUP_ENTRYPOINT:${JS_OUTPUT_FILE}" -c "${ROLLUP_CONFIG}" --preserveSymlinks

echo "Running terser"
TERSER_CONFIG_FILE="${REPO_ROOT}/hack/ts.rollup_bundle.min.minify_options.json"
TERSER_OUT_FILE="${BUNDLE_OUTPUT_DIR}/bundle.min.js"
./node_modules/.bin/terser "${ROLLUP_OUT_FILE}" --output "${TERSER_OUT_FILE}" --config-file "${TERSER_CONFIG_FILE}"

if [[ -n "${OUT:-}" ]]; then
    echo "Output is at ${OUT}"
    cp "${TERSER_OUT_FILE}" "${OUT}"
fi
