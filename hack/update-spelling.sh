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

if [[ -n "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then
  echo "Updating spelling..." >&2
elif ! command -v bazel &>/dev/null; then
  echo "Install bazel at https://bazel.build" >&2
  exit 1
else
  (
    set -o xtrace
    bazel run --test_output=streamed @io_k8s_test_infra//hack:update-spelling
  )
  exit 0
fi

find -L . -type f -not \( \
  \( \
    -path '*/vendor/*' \
    -o -path '*/static/*' \
    -o -path '*/third_party/*' \
    -o -path '*/node_modules/*' \
    -o -path '*/localdata/*' \
    \) -prune \
  \) -exec "$@" '{}' +
