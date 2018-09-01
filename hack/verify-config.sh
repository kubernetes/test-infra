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
JOBS_DIR="${TESTINFRA_ROOT}/config/jobs"
TMP_CONFIG=$(mktemp)
TMP_GENERATED_JOBS=$(mktemp)

trap 'rm $TMP_CONFIG && rm $TMP_GENERATED_JOBS' EXIT
cp "${PROW_CONFIG}" "${TMP_CONFIG}"

bazel run //config/jobs/kubernetes-security:genjobs -- \
"--config=${PROW_CONFIG}" \
"--jobs=${JOBS_DIR}" \
"--output=${TMP_GENERATED_JOBS}"

DIFF=$(diff "${TMP_GENERATED_JOBS}" "${JOBS_DIR}/kubernetes-security/generated-security-jobs.yaml")
if [ ! -z "$DIFF" ]; then
    echo "config is not correct, please run \\\`hack/update-config.sh\\\`"
    exit 1
fi
