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
source hack/build/setup-go.sh

readonly KO_DATA_PATH="testgrid/cmd/transfigure/kodata"

# Roll up typescripts
if [[ "${1:-}" == "--cleanup" ]]; then
    echo "Running in cleanup mode, no op"
    exit 0
fi

if [[ -d $KO_DATA_PATH ]]; then
    rm -rf $KO_DATA_PATH
fi
mkdir -p $KO_DATA_PATH

cp testgrid/cmd/transfigure/transfigure.sh "${KO_DATA_PATH}/"
go build -o "${KO_DATA_PATH}/configurator" ./testgrid/cmd/configurator
go build -o "${KO_DATA_PATH}/pr-creator" ./robots/pr-creator
