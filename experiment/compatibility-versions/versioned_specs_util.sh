#!/usr/bin/env bash
# Copyright 2026 The Kubernetes Authors.
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

set -o errexit -o nounset -o pipefail

# get_target_spec takes two arguments: specs_json and emulated_version
# Returns the highest spec strictly less than or equal to the emulated_version.
function get_target_spec() {
  local specs_json="$1"
  local emulated_version="$2"

  echo "${specs_json}" | jq -r --arg ver "${emulated_version}" '
        [ .[]
          | select(
              ( .version | sub("^v"; "") | tonumber )
              <=
              ($ver | sub("^v"; "") | tonumber)
            )
        ]
        | last
      '
}

# get_prev_target_spec takes two arguments: specs_json and emulated_version
# Returns the spec immediately preceding target_spec in the original sorted array.
function get_prev_target_spec() {
  local specs_json="$1"
  local emulated_version="$2"

  echo "${specs_json}" | jq -r --arg ver "${emulated_version}" '
        . as $arr |
        [ range(length)
          | select(
              ( $arr[.] | .version | sub("^v"; "") | tonumber )
              <=
              ($ver | sub("^v"; "") | tonumber)
            )
        ] | last as $idx
        | if $idx != null and $idx > 0 then $arr[$idx - 1] else null end
      '
}

# get_version_1_0_spec takes one argument: specs_json
# Returns the spec corresponding exactly to version 1.0.
function get_version_1_0_spec() {
  local specs_json="$1"
  get_exact_version_spec "$specs_json" "1.0"
}

# get_exact_version_spec takes two arguments: specs_json and target_version
# Returns the spec uniquely matching the exact target_version.
function get_exact_version_spec() {
  local specs_json="$1"
  local target_version="$2"

  echo "${specs_json}" | jq -r --arg ver "${target_version}" '
        [ .[]
          | select(
              ( .version | sub("^v"; "") | tonumber )
              ==
              ($ver | sub("^v"; "") | tonumber)
            )
        ]
        | if length > 0 then .[0] else null end
      '
}
