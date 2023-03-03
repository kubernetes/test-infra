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

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd "${REPO_ROOT}"

# Ensure virtual env
# Trick from https://pythonspeed.com/articles/activate-virtualenv-dockerfile/
export VIRTUAL_ENV="${REPO_ROOT}/.python_virtual_env"

if [[ ! -f "${VIRTUAL_ENV}/bin/activate" ]]; then
    python3 -m venv "${VIRTUAL_ENV}"
fi

source "${VIRTUAL_ENV}/bin/activate"
