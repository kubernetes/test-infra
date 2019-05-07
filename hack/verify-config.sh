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
  echo "Validating job configs..." >&2
elif ! command -v bazel &> /dev/null; then
  echo "Install bazel at https://bazel.build" >&2
  exit 1
else
  (
    set -o xtrace
    bazel test --test_output=streamed //hack:verify-config
  )
  exit 0
fi

trap 'echo ERROR: security jobs changed, run hack/update-config.sh >&2' ERR

echo -n "Running checkconfig with strict warnings..." >&2
"$@" \
    --strict \
    --warnings=mismatched-tide-lenient \
    --warnings=tide-strict-branch \
    --warnings=needs-ok-to-test \
    --warnings=validate-owners \
    --warnings=missing-trigger \
    --warnings=validate-urls \
    --warnings=unknown-fields
echo PASS

echo -n "Checking generated security jobs..." >&2
d=$(diff config/jobs/kubernetes-security/generated-security-jobs.yaml hack/zz.security-jobs.yaml || true)
if [[ -n "$d" ]]; then
  echo "FAIL" >&2
  echo "< unexpected" >&2
  echo "> missing" >&2
  echo "$d" >&2
  false
fi
