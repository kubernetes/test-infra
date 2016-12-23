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

set -o errexit
set -o nounset
set -o pipefail

# Cache location.
export TEST_TMPDIR="/root/.cache/bazel"

bazel build //cmd/... //pkg/... //federation/... //plugin/... //build/... //examples/... //test/... //third_party/... && rc=$? || rc=$?

# Clear test.xml so that we don't pick up old results.
find -L bazel-testlogs -name 'test.xml' -type f -exec rm '{}' +

if [[ "${rc}" == 0 ]]; then
  bazel test  --test_output=errors --test_tag_filters '-skip' //cmd/... //pkg/... //federation/... //plugin/... //build/... //third_party/... && rc=$? || rc=$?
fi

if [[ "${rc}" == 0 ]]; then
  bazel run //:ci-artifacts -- "gs://kubernetes-release-dev/bazel/$(git rev-parse HEAD)" && rc=$? || rc=$?
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

# Coalesce test results into one file for upload.
"$(dirname "${0}")/../images/pull-kubernetes-bazel/coalesce.py"

exit "${rc}"
