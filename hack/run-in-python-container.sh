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
PYLINTHOME="$(mktemp -d)"

# Technically we can assume that this is always set, setting here again to
# ensure consistency
VIRTUAL_ENV="${VIRTUAL_ENV:-${REPO_ROOT:?}/.python_virtual_env}"

# Setting PATH is a trick learnt from
# https://pythonspeed.com/articles/activate-virtualenv-dockerfile/, for
# mimicking the behavior of `source venv_dir/bin/activate`, same as setting
# PYTHONPATH to empty. Tried several other alternatives:
#   - Set PATH in docker image. This doesn't work because the workspace is set
#     afterwards.
#   - Run `source venv_dir/bin/activate` as part of docker run argument. This
#     cannot be done directly as `source` is a bash builtin, so have to do `bash
#     -c `source.., python3..` stuff, and the quotes passing around almost
#     killed me.
docker run \
    --rm -i \
    -e HOME=/tmp \
    -e PYLINTHOME=${PYLINTHOME} \
    -e VIRTUAL_ENV="${VIRTUAL_ENV}" \
    -e PATH="${VIRTUAL_ENV}/bin:/usr/local/bin:/bin" \
    -e PYTHONPATH='' \
    -v "${PYLINTHOME}:${PYLINTHOME}" \
    -v "${REPO_ROOT:?}:${REPO_ROOT:?}" -w "${REPO_ROOT}" \
    "${PY_IMAGE}" \
    "$@"
