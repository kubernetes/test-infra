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
cd $REPO_ROOT

# place to stick temp binaries
BINDIR="${REPO_ROOT}/_bin"

DIFFROOT="${REPO_ROOT}/prow"
TMP_DIFFROOT="$(TMPDIR="${BINDIR}" mktemp -d "${BINDIR}/verify-codegen.XXXXX")"

cp -a "${DIFFROOT}"/{apis,client,config,spyglass} "${TMP_DIFFROOT}"

"${REPO_ROOT}/hack/make-rules/update/codegen.sh"

echo "diffing ${DIFFROOT} against freshly generated codegen"
ret=0
diff -Naupr "${DIFFROOT}/apis" "${TMP_DIFFROOT}/apis" || ret=$?
diff -Naupr "${DIFFROOT}/client" "${TMP_DIFFROOT}/client" || ret=$?
diff -Naupr "${DIFFROOT}/config" "${TMP_DIFFROOT}/config" || ret=$?
diff -Naupr "${DIFFROOT}/spyglass" "${TMP_DIFFROOT}/spyglass" || ret=$?
# Restore so that verify codegen doesn't modify workspace
cp -a "${TMP_DIFFROOT}"/{apis,client,config,spyglass} "${DIFFROOT}"
# Clean up
rm -rf "${TMP_DIFFROOT}"

if [[ ${ret} -eq 0 ]]; then
  echo "${DIFFROOT} up to date."
  exit 0
fi
echo "ERROR: out of date codegen files. Fix with make update-codegen" >&2
exit 1
