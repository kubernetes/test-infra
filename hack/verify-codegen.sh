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

if [[ -n "${TEST_WORKSPACE:-}" ]]; then # Running inside bazel
  echo "Validating codegen files..." >&2
elif ! command -v bazel &> /dev/null; then
  echo "Install bazel at https://bazel.build" >&2
  exit 1
else
  (
    set -o xtrace
    bazel test --test_output=streamed @io_k8s_test_infra//hack:verify-codegen
  )
  exit 0
fi

SCRIPT_ROOT=$PWD

DIFFROOT="${SCRIPT_ROOT}/prow"
TMP_DIFFROOT="${TEST_TMPDIR}/prow"

mkdir -p "${TMP_DIFFROOT}"
cp -a "${DIFFROOT}"/{apis,client,spyglass} "${TMP_DIFFROOT}"

clean=yes # bazel test files are read-only, must first delete
BUILD_WORKSPACE_DIRECTORY="$SCRIPT_ROOT" "$@" "$clean"
echo "diffing ${DIFFROOT} against freshly generated codegen"
ret=0
diff -Naupr "${DIFFROOT}/apis" "${TMP_DIFFROOT}/apis" || ret=$?
diff -Naupr "${DIFFROOT}/client" "${TMP_DIFFROOT}/client" || ret=$?
diff -Naupr "${DIFFROOT}/spyglass" "${TMP_DIFFROOT}/spyglass" || ret=$?
cp -a "${TMP_DIFFROOT}"/{apis,client,spyglass} "${DIFFROOT}"
if [[ ${ret} -eq 0 ]]; then
  echo "${DIFFROOT} up to date."
  exit 0
fi
echo "ERROR: out of date codegen files. Fix with hack/update-codegen.sh" >&2
exit 1
