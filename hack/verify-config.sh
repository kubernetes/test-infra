#!/usr/bin/env bash
# Copyright 2018 The Kubernetes Authors.
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
set -o xtrace

TESTINFRA_ROOT=$(git rev-parse --show-toplevel)

PROW_CONFIG="${TESTINFRA_ROOT}/prow/config.yaml"
JOBS_CONFIG="${TESTINFRA_ROOT}/jobs/config.json"
TMP_CONFIG=$(mktemp)
trap "rm $TMP_CONFIG" EXIT
cp "${PROW_CONFIG}" "${TMP_CONFIG}"
bazel run //maintenance/fixconfig:fixconfig -- --config=${TMP_CONFIG} --config-json=${JOBS_CONFIG} && \
bazel run //jobs:config_sort -- --prow-config=${TMP_CONFIG} --only-prow

DIFF=$(diff "${TMP_CONFIG}" "${PROW_CONFIG}")
if [ ! -z "$DIFF"]; then
    echo "config is not correct, please run `hack/update-config.sh`"
    exit 1
fi
