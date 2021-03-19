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

if [[ -n "${TEST_WORKSPACE:-}" ]]; then
  echo "Validating spelling..." >&2
elif ! command -v bazel &> /dev/null; then
  echo "Install bazel at https://bazel.build" >&2
  exit 1
else
  (
    set -o xtrace
    bazel test --test_output=streamed @io_k8s_test_infra//hack:verify-spelling
  )
  exit 0
fi

trap 'echo ERROR: found unexpected instance of "Git"hub, use github or GitHub' ERR

# Unit test: Git"hub (remove ")
# Appear to need to use this if statement on mac to get the not grep to work
if find -L . -type f -not \( \
  \( \
    -path '*/vendor/*' \
    -o -path '*/external/*' \
    -o -path '*/static/*' \
    -o -path '*/third_party/*' \
    -o -path '*/node_modules/*' \
    -o -path '*/localdata/*' \
    -o -path '*/gubernator/*' \
    -o -path '*/prow/bugzilla/client_test.go' \
    \) -prune \
    \) -exec grep -Hn 'Git'hub '{}' '+' ; then
  false
fi


trap 'echo ERROR: bad spelling, fix with hack/update-spelling.sh' ERR

# Unit test: lang auge (remove space)
find -L . -type f -not \( \
  \( \
    -path '*/vendor/*' \
    -o -path '*/external/*' \
    -o -path '*/static/*' \
    -o -path '*/third_party/*' \
    -o -path '*/node_modules/*' \
    -o -path '*/localdata/*' \
    \) -prune \
    \) -exec "$@" '{}' '+'

echo 'PASS: No spelling issues detected'
