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

FAILED=()
if [[ "${VERIFY_GO_LINT:-true}" == "true" ]]; then
  name="go lints"
  echo "verifying $name ..."
  hack/make-rules/verify/golangci-lint.sh || { FAILED+=($name); echo "ERROR: $name failed"; }
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_GOFMT:-true}" == "true" ]]; then
  name="go fmt"
  echo "verifying $name"
  hack/make-rules/verify/gofmt.sh || { FAILED+=($name); echo "ERROR: $name failed"; }
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_FILE_PERMS:-true}" == "true" ]]; then
  name=".sh files permissions"
  echo "verifying $name"
  hack/make-rules/verify/file-perms.sh || { FAILED+=($name); echo "ERROR: $name failed"; }
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_SPELLING:-true}" == "true" ]]; then
  name="spelling"
  echo "verifying $name"
  hack/make-rules/verify/misspell.sh || { FAILED+=($name); echo "ERROR: $name failed"; }
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_LABELS:-true}" == "true" ]]; then
  name="labels"
  echo "verifying $name"
  hack/make-rules/verify/labels.sh || { FAILED+=($name); echo "ERROR: $name failed"; }
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_ESLINT:-true}" == "true" ]]; then
  name="eslint"
  echo "verifying $name"
  hack/make-rules/verify/eslint.sh || { FAILED+=($name); echo "ERROR: $name failed"; }
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_PYLINT:-true}" == "true" ]]; then
  name="pylint"
  echo "verifying $name"
  hack/make-rules/verify/pylint.sh || { FAILED+=($name); echo "ERROR: $name failed"; }
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_BOILERPLATE:-true}" == "true" ]]; then
  name="boilerplate"
  echo "verifying $name"
  hack/make-rules/verify/boilerplate.sh || { FAILED+=($name); echo "ERROR: $name failed"; }
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_YAMLLINT:-true}" == "true" ]]; then
  name="yamllint"
  echo "verifying $name"
  hack/make-rules/verify/yamllint.sh || { FAILED+=($name); echo "ERROR: $name failed"; }
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_TS_ROLLUP:-true}" == "true" ]]; then
  name="rollup typescripts"
  echo "verifying $name"
  hack/make-rules/verify/ts-rollup.sh || { FAILED+=($name); echo "ERROR: $name failed"; }
  cd "${REPO_ROOT}"
fi
if [[ "${VERIFY_GO_DEPS:-true}" == "true" ]]; then
  name="go deps"
  echo "verifying $name"
  hack/make-rules/verify/go-deps.sh || { FAILED+=($name); echo "ERROR: $name failed"; }
  cd "${REPO_ROOT}"
fi

# exit based on verify scripts
if [[ "${#FAILED[@]}" == 0 ]]; then
  echo ""
  echo "All verify checks passed, congrats!"
else
  echo ""
  echo "One or more verify checks failed! See details above. Failed check:"
  for failed in "${FAILED}"; do
    echo "  FAILED: $failed"
  done
  res=1
fi
exit "${res}"
