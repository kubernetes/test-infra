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

# Remove //vendor/golang.org/x/text/internal:go_default_library deps
#
# Unused text/gen.go file imports text/internal, which prevents dep prune from
# removing it. However text/internal package references a text/language package
# which has been pruned.
#
# Fix by deleting this dependency. The target won't build correctly, but it
# won't anyway because text/language doesn't exist, and it will get pruned by
# hack/prune-libraries.sh
#
# Usage:
#   remove-text-internal
patch-text-internal() {
  local path
  path="$(dirname "${BASH_SOURCE}")/../vendor/golang.org/x/text/internal/BUILD"
  if [[ ! -f ${path} ]]; then
    return 0
  fi
  sed -i -e "\|//vendor/golang.org/x/text/language:go_default_library|d" "$path"
}

main() {
  pushd "$(dirname "${BASH_SOURCE}")/.."
  dep ensure
  dep prune
  hack/update-bazel.sh
  patch-text-internal  # TODO(fejta): fix dep prune instead
  hack/prune-libraries.sh --fix
  hack/update-bazel.sh  # Update child :all-srcs in case parent was deleted
}

if ! main; then
  echo FAILED >&2
  exit 1
fi
echo SUCCESS
