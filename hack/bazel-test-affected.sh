#!/bin/bash
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

# Usage:
# The script builds and run tests affected by modified files.
#
# The list of modified files is created based on the git-diff options passed
# as parameters to this command.
#
# Examples:
# bazel-test-affected.sh HEAD     # Default: Use unstaged/uncommitted changes
# bazel-test-affected.sh --staged # Check build/tests affected by staged files
# bazel-test-affected.sh origin/master... # Check files changed off master
# bazel-test-affected.sh HEAD~ # Check build/tests affected by previous commit
#
# Please refer to `man git-diff` for more possible syntax.

set -o errexit
set -o nounset
set -o pipefail

# Compute list of modified files in bazel package form.
packages=($(bazel query \
  --noshow_progress \
  "set($(git diff --name-only --diff-filter=d "${@}"))"))
if [[ "${#packages[@]}" == 0 ]]; then
  echo "No bazel packages affected."
  exit 0
fi

# Build modified packages.
buildables=$(bazel query \
  --keep_going \
  --noshow_progress \
  "kind(.*_binary, rdeps(//..., set(${packages[@]})))")
if [[ ! -z "${buildables}" ]]; then
  bazel build ${buildables}
fi

# Run affected tests.
tests=$(bazel query \
  --keep_going \
  --noshow_progress \
  "kind(test, rdeps(//..., set(${packages[@]})))")
bazel test --test_output=errors ${tests}
