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

set -o nounset
set -o errexit
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd $REPO_ROOT

# exit code, if a script fails we'll set this to 1
res=0

# run all verify scripts, optionally skipping any of them

if [[ "${VERIFY_GO_LINT:-true}" == "true" ]]; then
  echo "verifying go lints ..."
  hack/make-rules/verify/golangci-lint.sh || res=1
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_GOFMT:-true}" == "true" ]]; then
  echo "verifying go fmt ..."
  hack/make-rules/verify/gofmt.sh || res=1
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_FILE_PERMS:-true}" == "true" ]]; then
  echo "verifying .sh files permissions ..."
  hack/make-rules/verify/file-perms.sh || res=1
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_SPELLING:-true}" == "true" ]]; then
  echo "verifying spelling ..."
  hack/make-rules/verify/misspell.sh || res=1
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_LABELS:-true}" == "true" ]]; then
  echo "verifying labels ..."
  hack/make-rules/verify/labels.sh || res=1
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_CODEGEN:-true}" == "true" ]]; then
  echo "verifying codegen ..."
  hack/make-rules/verify/codegen.sh || res=1
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_TSLINT:-true}" == "true" ]]; then
  echo "verifying tslint ..."
  hack/make-rules/verify/tslint.sh || res=1
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_PYLINT:-true}" == "true" ]]; then
  echo "verifying pylint ..."
  hack/make-rules/verify/pylint.sh || res=1
  cd "${REPO_ROOT}"
fi

# exit based on verify scripts
if [[ "${res}" = 0 ]]; then
  echo ""
  echo "All verify checks passed, congrats!"
else
  echo ""
  echo "One or more verify checks failed! See output above..."
fi
exit "${res}"
