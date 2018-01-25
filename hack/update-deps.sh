#!/bin/bash
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


# Identifies go_library() bazel targets in the workspace with no dependencies.
# Deletes the target and its files. Cleans up any :all-srcs references

# By default just checks for extra libraries, run with --fix to make changes.

set -o nounset
set -o errexit
set -o pipefail
set -o xtrace

trap 'echo "FAILED" >&2' ERR
pushd "$(dirname "${BASH_SOURCE}")/.."
dep ensure -v
hack/update-bazel.sh
hack/prune-libraries.sh --fix
hack/update-bazel.sh  # Update child :all-srcs in case parent was deleted
echo SUCCESS
