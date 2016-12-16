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

# TODO(fejta): remove this
case "${PULL_BASE_REF:-}" in
release-1.0|release-1.1|release-1.2|release-1.3|release-1.4|release-1.5)
  echo "PR Kubemark e2e GCE job disabled for legacy branches."
  exit 0
  ;;
esac

bazel build //cmd/... //pkg/... //federation/... //plugin/... //build/... //examples/... //test/... //third_party/... && rc=$? || rc=$?

if [[ "${rc}" == 0 ]]; then
  bazel test --test_tag_filters '-skip' //cmd/... //pkg/... //federation/... //plugin/... //build/... //third_party/... && rc=$? || rc=$?
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

# Copy bazel-testlogs/foo/bar/go_default_test/test.xml into _artifacts/foo_bar.xml.
mkdir -p _artifacts
for file in $(find -L bazel-testlogs -name "test.xml" -o -name "test.log"); do
    cp "${file}" "_artifacts/$(echo "${file}" | sed -e "s/bazel-testlogs\///g; s/\/go_default_test\/test//g; s/\//_/g")"
done

exit "${rc}"
