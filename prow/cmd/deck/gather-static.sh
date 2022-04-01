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

# Build ts-rollup so that it can run in docker
source hack/build/setup-go.sh
if [[ -z ${NO_DOCKER:-} ]]; then
  GOOS=linux GOARCH=amd64 go build -o _bin/ts-rollup k8s.io/test-infra/hack/ts-rollup
else
  go build -o _bin/ts-rollup k8s.io/test-infra/hack/ts-rollup
fi

readonly STATIC_MAP_FILE="prow/cmd/deck/static-map"
readonly JS_OUTPUT_DIR="_output/js"
mkdir -p "${JS_OUTPUT_DIR}"
readonly KO_DATA_PATH="prow/cmd/deck/kodata"
if [[ -d $KO_DATA_PATH ]]; then
    rm -rf $KO_DATA_PATH
fi

# Clean if meant to be
if [[ "${1:-}" == "--cleanup" ]]; then
    echo "Running in cleanup mode"
    ./hack/run-in-node-container.sh _bin/ts-rollup --packages="${REPO_ROOT}/prow/cmd/deck/.ts-packages" --root-dir=. --cleanup-only
    rm -rf ${KO_DATA_PATH}
    exit 0
fi

# ensure deps are installed
./hack/build/ensure-node_modules.sh

# Roll up typescripts
./hack/run-in-node-container.sh _bin/ts-rollup --packages="${REPO_ROOT}/prow/cmd/deck/.ts-packages" --root-dir=.

STATIC_MAP=()
while IFS= read -r map; do
    STATIC_MAP+=("${map}")
done < "${STATIC_MAP_FILE}"

for map in "${STATIC_MAP[@]}"; do
    parts=(${map//->/ })
    src="${REPO_ROOT}/${parts[0]}"
    dst="${KO_DATA_PATH}/${parts[1]}"
    echo "src: $src, dst: $dst"
    mkdir -p $dst
    rsync \
        -av \
        --exclude='*.go' \
        --exclude='*.ts' \
        --exclude='tsconfig.json' \
        --exclude='*.bazel' \
        --exclude='*/*.go' \
        --exclude='*/*.ts' \
        --exclude='*/tsconfig.json' \
        --exclude='*/*.bazel' \
        $src \
        $dst
done
