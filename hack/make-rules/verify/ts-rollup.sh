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

TS_PACKAGES_FILES=(
    # Only verify gopherage, as deck is already verified by `make -C prow
    # build-images`.
    # "prow/cmd/deck/.ts-packages"
    "gopherage/.ts-packages"
)

for ts_packages_files in "${TS_PACKAGES_FILES[@]}"; do
    ./hack/make-rules/update/ts-rollup.sh "${ts_packages_files}"
    CLEAN=true ./hack/make-rules/update/ts-rollup.sh "${ts_packages_files}"
done
