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

# This script runs $@ in a python container

set -o errexit
set -o nounset
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${REPO_ROOT}"

PY_IMAGE='python:3.9-slim-buster'

docker run \
    --rm -i \
    -e HOME=/tmp \
    -e PYTHONPATH='' \
    -v "${REPO_ROOT:?}:${REPO_ROOT:?}" -w "${REPO_ROOT}" \
    --security-opt="label=disable" \
    "${PY_IMAGE}" \
    bash -c 'source ./hack/make-rules/py-test/activate-python_venv.sh && $0 $@' "$@"
