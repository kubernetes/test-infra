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

DIR=$( cd "$( dirname "$0" )" && pwd )

if [[ -n "${TEST_WORKSPACE:-}" ]]; then # Running inside bazel
  echo "Linting python..." >&2
elif ! command -v bazel &> /dev/null; then
  echo "Install bazel at https://bazel.build" >&2
  exit 1
else
  (
    set -o xtrace
    bazel test --test_output=streamed //hack:verify-pylint
  )
  exit 0
fi

export PYLINTHOME=$TEST_TMPDIR

shopt -s extglob globstar

# TODO(clarketm) there is no version of `pylint` that supports "both" PY2 and PY3
# I am disabling pylint checks for python3 files until migration complete
"$DIR/pylint_bin" !(triage|velodrome|hack|gubernator|external|vendor|bazel-*)/**/*.py
