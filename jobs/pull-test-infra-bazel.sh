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
set -o xtrace

# TODO(fejta): bake dependencies into bazel rather than installing them via pip
test_requirements=(
  gubernator/test_requirements.txt
  jenkins/test_history/requirements.txt  # TODO(fejta): remove
  verify/test_requirements.txt
)
for path in ${test_requirements[@]}; do
  if [[ ! -f "${path}" ]]; then
    continue
  fi
  pip install -r "${path}"
done

# Cache location.
export TEST_TMPDIR="/root/.cache/bazel"

# Run a no-target bazel to create the `bazel-testlogs` symlink.
# We need the symlink to be created so that we can remove left-over from
# previous tests (the symlink goes to a cache that is shared between runs).
bazel build
find -L bazel-testlogs -name 'test.xml' -type f -exec rm '{}' +

verify/bazel-test-affected.sh "${PULL_BASE_SHA}...${PULL_PULL_SHA}" && rc=$? || rc=$?

./images/pull_kubernetes_bazel/coalesce.py

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
