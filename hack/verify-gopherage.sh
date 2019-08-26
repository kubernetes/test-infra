#!/usr/bin/env bash
# Copyright 2019 The Kubernetes Authors.
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
  echo "Verifying gopherage scripts..." >&2
elif ! command -v bazel &>/dev/null; then
  echo "Install bazel at https://bazel.build" >&2
  exit 1
else
  (
    set -o xtrace
    bazel test --test_output=streamed //hack:verify-gopherage
  )
  exit 0
fi

trap 'echo ERROR: gopherage static content changed, run hack/update-gopherage.sh >&2' ERR

echo -n "Checking generated security jobs..." >&2
d=$(diff gopherage/cmd/html/static/zz.browser_bundle.es2015.js gopherage/cmd/html/static/browser_bundle.es2015.js || true)
if [[ -n "$d" ]]; then
  echo "FAIL" >&2
  echo "< unexpected" >&2
  echo "> missing" >&2
  echo "$d" >&2
  false
fi
