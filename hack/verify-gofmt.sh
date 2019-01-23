#!/usr/bin/env bash
# Copyright 2017 The Kubernetes Authors.
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

cmd="bazel run //:gofmt --"
if ! which bazel &> /dev/null; then
  echo "Bazel is the preferred way to build and test the test-infra repo." >&2
  echo "Please install bazel at https://bazel.build/ (future commits may require it)" >&2
  cmd="gofmt"
fi
diff=$(find . -name "*.go" | grep -v "\/vendor\/" | xargs ${cmd} -s -d)
if [[ -n "${diff}" ]]; then
  echo "${diff}"
  echo
  echo "Please run hack/update-gofmt.sh"
  exit 1
fi
