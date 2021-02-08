#!/bin/bash
# Copyright 2021 The Kubernetes Authors.
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

# check-file ensures that all static links are valid
check-file() {
  for path in $(grep -h -o -E '/static[^"]+' $1); do
      local target=prow/cmd/deck$path
      if [[ ! -f "$target" ]]; then
          echo "  ERROR: $path not found at $target in $1" >&2
          return 1
      else
          echo "  found $path link to $target"
      fi
  done
}

fail=()
for arg in "$@"; do
    echo Validating "$arg":
    check-file "$arg" && echo "  PASS: $arg" || fail+=("$arg")
done
if [[ "${#fail[@]}" -eq 0 ]]; then
    exit 0
fi
echo "FAIL: bad links in the following files:" >&2
for bad in "${fail[@]}"; do
    echo "  $bad" >&2
done
exit 1
