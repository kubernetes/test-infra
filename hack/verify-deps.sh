#!/bin/bash
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

set -o nounset
set -o errexit
set -o pipefail

TESTINFRA_ROOT=$(git rev-parse --show-toplevel)
cd "${TESTINFRA_ROOT}"

_tmpdir="$(mktemp -d -t verify-deps.XXXXXX)"
cd "${_tmpdir}"
_tmpdir="$(pwd)"

trap "rm -rf ${_tmpdir}" EXIT

_tmp_gopath="${_tmpdir}/go"
_tmp_testinfra_root="${_tmp_gopath}/src/k8s.io/test-infra"
mkdir -p "${_tmp_testinfra_root}/.."
cp -a "${TESTINFRA_ROOT}" "${_tmp_testinfra_root}/.."

cd "${_tmp_testinfra_root}"
GOPATH="${_tmp_gopath}" PATH="${_tmp_gopath}/bin:${PATH}" ./hack/update-deps.sh

diff=$(diff -Nupr \
  -x ".git" \
  -x "bazel-*" \
  -x "_output" \
  "${TESTINFRA_ROOT}" "${_tmp_testinfra_root}" 2>/dev/null || true)

if [[ -n "${diff}" ]]; then
  echo "${diff}" >&2
  echo >&2
  echo "Run ./hack/update-deps.sh" >&2
  exit 1
fi
