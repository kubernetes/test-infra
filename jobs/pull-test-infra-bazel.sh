#!/bin/bash
# Copyright 2016 The Kubernetes Authors.
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

# Build and run only affected targets.
# Logic adapted from https://github.com/bazelbuild/bazel/blob/633e48a4ce8747575b156639608ee18afe4b386e/scripts/ci/ci.sh

set -o errexit
set -o nounset
set -o pipefail

pip install -r gubernator/test_requirements.txt
pip install -r jenkins/test_history/requirements.txt

# Cache location.
export TEST_TMPDIR="/root/.cache/bazel"

# Compute list of modified files in bazel package form.
commit_range="${PULL_BASE_SHA}..${PULL_PULL_SHA}"
files=()
# git diff --name-only only prints the original (old) filename for renames
# git diff --name-status prints the new name followed by the old name for
# renames, so use it instead (after cutting out the first field, the status)
for file in $(git diff --name-status --diff-filter=d "${commit_range}" | cut -f2); do
  files+=($(bazel query "${file}"))
done

# Build modified packages.
buildables=$(bazel query \
    --keep_going \
    --noshow_progress \
    "kind(.*_binary, rdeps(//..., set(${files[@]})))")
rc=0
if [[ ! -z "${buildables}" ]]; then
  bazel build ${buildables} && rc=$? || rc=$?
  # Clear test.xml so that we don't pick up old results.
  find -L bazel-testlogs -name 'test.xml' -type f -exec rm '{}' +
fi

# Run affected tests.
if [[ "${rc}" == 0 ]]; then
  tests=$(bazel query \
      --keep_going \
      --noshow_progress \
      "kind(test, rdeps(//..., set(${files[@]}))) except attr('tags', 'manual', //...)")
  bazel test --test_output=errors ${tests} //verify:verify-all && rc=$? || rc=$?
  ./images/pull_kubernetes_bazel/coalesce.py
fi

case "${rc}" in
    0) echo "Success" ;;
    1) echo "Build failed" ;;
    2) echo "Bad environment or flags" ;;
    3) echo "Build passed, tests failed or timed out" ;;
    4) echo "Build passed, no tests found" ;;
    5) echo "Interrupted" ;;
    *) echo "Unknown exit code: ${rc}" ;;
esac

exit "${rc}"
