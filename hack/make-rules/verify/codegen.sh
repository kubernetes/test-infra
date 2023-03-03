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
if [[ ! -d "${BINDIR}" ]]; then
  mkdir "${BINDIR}"
fi

DIFFROOT="${REPO_ROOT}"
TMP_DIFFROOT="$(TMPDIR="${BINDIR}" mktemp -d "${BINDIR}/verify-codegen.XXXXX")"

mkdir -p "${TMP_DIFFROOT}/prow"
cp -a "${DIFFROOT}"/prow/{apis,client,config,gangway,plugins} "${TMP_DIFFROOT}/prow"
mkdir -p "${TMP_DIFFROOT}/config/prow/cluster/prowjob-crd"
cp -a "${DIFFROOT}/config/prow/cluster/prowjob-crd/prowjob_customresourcedefinition.yaml" "${TMP_DIFFROOT}/config/prow/cluster/prowjob-crd/prowjob_customresourcedefinition.yaml"

"${REPO_ROOT}/hack/make-rules/update/codegen.sh"

echo "diffing ${DIFFROOT} against freshly generated codegen"
ret=0
diff -Naupr "${DIFFROOT}/prow/apis" "${TMP_DIFFROOT}/prow/apis" || ret=$?
diff -Naupr "${DIFFROOT}/prow/client" "${TMP_DIFFROOT}/prow/client" || ret=$?
diff -Naupr "${DIFFROOT}/prow/config" "${TMP_DIFFROOT}/prow/config" || ret=$?
diff -Naupr "${DIFFROOT}/prow/gangway" "${TMP_DIFFROOT}/prow/gangway" || ret=$?
diff -Naupr "${DIFFROOT}/config/prow/cluster/prowjob-crd/prowjob_customresourcedefinition.yaml" "${TMP_DIFFROOT}/config/prow/cluster/prowjob-crd/prowjob_customresourcedefinition.yaml" || ret=$?
# Restore so that verify codegen doesn't modify workspace
cp -a "${TMP_DIFFROOT}/prow"/{apis,client,config} "${DIFFROOT}"/prow
cp -a "${TMP_DIFFROOT}/config/prow/cluster/prowjob-crd/prowjob_customresourcedefinition.yaml" "${DIFFROOT}/config/prow/cluster/prowjob-crd/prowjob_customresourcedefinition.yaml"

# Clean up
rm -rf "${TMP_DIFFROOT}"

if [[ ${ret} -eq 0 ]]; then
  echo "${DIFFROOT} up to date."
  exit 0
fi
echo "ERROR: out of date codegen files. Fix with make update-codegen" >&2
exit 1
