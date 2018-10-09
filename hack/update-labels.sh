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
LABELS_CONFIG=${LABELS_CONFIG:-"${TESTINFRA_ROOT}/label_sync/labels.yaml"}
LABELS_DOCS_TEMPLATE=${LABELS_DOCS_TEMPLATE:-"${TESTINFRA_ROOT}/label_sync/labels.md.tmpl"}
LABELS_DOCS_OUTPUT=${LABELS_DOCS_OUTPUT:-"${TESTINFRA_ROOT}/label_sync/labels.md"}
LABELS_CSS_TEMPLATE=${LABELS_CSS_TEMPLATE:-"${TESTINFRA_ROOT}/label_sync/labels.css.tmpl"}
LABELS_CSS_OUTPUT=${LABELS_CSS_OUTPUT:-"${TESTINFRA_ROOT}/prow/cmd/deck/static/labels.css"}

bazel run //label_sync -- \
--config=${LABELS_CONFIG} \
--action=docs \
--docs-template=${LABELS_DOCS_TEMPLATE} \
--docs-output=${LABELS_DOCS_OUTPUT} \
--css-template=${LABELS_CSS_TEMPLATE} \
--css-output=${LABELS_CSS_OUTPUT}
