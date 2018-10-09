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

TESTINFRA_ROOT=$(git rev-parse --show-toplevel)

TMP_LABELS_DOCS=$(mktemp)
TMP_LABELS_CSS=$(mktemp)
trap 'rm -f "${TMP_LABELS_DOCS}" & rm -f "${TMP_LABELS_CSS}"' EXIT
LABELS_DOCS_OUTPUT="${TMP_LABELS_DOCS}" LABELS_CSS_OUTPUT="${TMP_LABELS_CSS}" ${TESTINFRA_ROOT}/hack/update-labels.sh


DIFF=$(diff "${TMP_LABELS_DOCS}" "${TESTINFRA_ROOT}/label_sync/labels.md" || true)
if [ ! -z "$DIFF" ]; then
    echo "${DIFF}"
    echo ""
    echo "FAILED: labels.yaml was updated without updating labels.md, please run 'hack/update-labels.sh'"
    exit 1
fi

DIFF=$(diff "${TMP_LABELS_CSS}" "${TESTINFRA_ROOT}/prow/cmd/deck/static/labels.css" || true)
if [ ! -z "$DIFF" ]; then
    echo "${DIFF}"
    echo ""
    echo "FAILED: labels.yaml was updated without updating labels.css, please run 'hack/update-labels.sh'"
    exit 1
fi
