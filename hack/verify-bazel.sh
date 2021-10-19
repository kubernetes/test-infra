#!/usr/bin/env bash
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

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"

if ! command -v bazel &> /dev/null; then
  echo "Install bazel at https://bazel.build" >&2
  exit 1
fi

pushd "${REPO_ROOT}/hack/tools" >/dev/null
  GO111MODULE=on go install github.com/bazelbuild/buildtools/buildozer
popd >/dev/null
# make go_test targets manual
# buildozer exits 3 when no changes are made ¯\_(ツ)_/¯
# https://github.com/bazelbuild/buildtools/tree/master/buildozer#error-code
buildozer -quiet 'add tags manual' '//...:%go_binary' '//...:%go_test' && ret=$? || ret=$?
if [[ $ret != 0 && $ret != 3 ]]; then
  exit 1
fi

set -o xtrace
bazel test --test_output=streamed @io_k8s_repo_infra//hack:verify-bazel
