#!/usr/bin/env bash
# Copyright 2019 The Kubernetes Authors.
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

if [[ -n "${TEST_WORKSPACE:-}" ]]; then # Running inside bazel
  echo "Checking protos for changes..." >&2
elif ! command -v bazel &>/dev/null; then
  echo "Install bazel at https://bazel.build" >&2
  exit 1
else
  (
    set -o xtrace
    bazel test //hack:verify-protos
  )
  exit 0
fi

TESTINFRA_ROOT=$PWD

_tmpdir="$(mktemp -d -t verify-deps.XXXXXX)"
trap "rm -rf ${_tmpdir}" EXIT

cp -a "${TESTINFRA_ROOT}/." "${_tmpdir}"

# Update protos, outputting to $_tmpdir
(
  update_protos=$1
  protoc=$2
  plugin=$3
  boiler=$4
  grpc=$5
  importmap=$6

  export BUILD_WORKSPACE_DIRECTORY=${_tmpdir}
  "$update_protos" "$protoc" "$plugin" "$boiler" "$grpc" "$importmap"
)

# Ensure nothing changed
diff=$(diff -Nupr \
  -x ".git" \
  -x "bazel-*" \
  -x "_output" \
  "${TESTINFRA_ROOT}" "${_tmpdir}" 2>/dev/null || true)

if [[ -n "${diff}" ]]; then
  echo "${diff}" >&2
  echo >&2
  echo "ERROR: protos changed. Run bazel run //hack:update-protos" >&2
  exit 1
fi
