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

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")
source "${SCRIPT_DIR}/versioned_specs_util.sh"

run_tests() {
  local failures=0

  echo "Running tests for versioned_specs_util.sh"

  local mock_specs='[{"version":"1.30","preRelease":"Alpha"},{"version":"1.32","preRelease":"Beta"},{"version":"1.34","preRelease":"GA"}]'

  # Test get_target_spec
  local got
  got=$(get_target_spec "$mock_specs" "1.33")
  if [[ "$got" != *'"1.32"'* ]]; then
    echo "FAIL: get_target_spec expected 1.32, got: $got"
    failures=$((failures+1))
  else
    echo "PASS: get_target_spec"
  fi

  # Test get_prev_target_spec
  got=$(get_prev_target_spec "$mock_specs" "1.33")
  if [[ "$got" != *'"1.30"'* ]]; then
    echo "FAIL: get_prev_target_spec expected 1.30, got: $got"
    failures=$((failures+1))
  else
    echo "PASS: get_prev_target_spec"
  fi

  got=$(get_prev_target_spec "$mock_specs" "1.30")
  if [[ "$got" != "null" ]]; then
    echo "FAIL: get_prev_target_spec (first element) expected null, got: $got"
    failures=$((failures+1))
  else
    echo "PASS: get_prev_target_spec (first element)"
  fi

  # Test get_version_1_0_spec
  local mock_specs_1_0='[{"version":"1.0","preRelease":"GA"},{"version":"1.33","preRelease":"Deprecated"}]'
  got=$(get_version_1_0_spec "$mock_specs_1_0")
  if [[ "$got" != *'"1.0"'* ]]; then
    echo "FAIL: get_version_1_0_spec expected 1.0 spec, got: $got"
    failures=$((failures+1))
  else
    echo "PASS: get_version_1_0_spec"
  fi

  # Test get_exact_version_spec
  got=$(get_exact_version_spec "$mock_specs" "1.30")
  if [[ "$got" != *'"1.30"'* ]]; then
    echo "FAIL: get_exact_version_spec expected 1.30 spec, got: $got"
    failures=$((failures+1))
  else
    echo "PASS: get_exact_version_spec"
  fi

  got=$(get_exact_version_spec "$mock_specs" "1.31")
  if [[ "$got" != "null" ]]; then
    echo "FAIL: get_exact_version_spec for non-existent version expected null, got: $got"
    failures=$((failures+1))
  else
    echo "PASS: get_exact_version_spec for non-existent version"
  fi

  if [[ $failures -gt 0 ]]; then
    echo "Tests failed ($failures failures)."
    exit 1
  else
    echo "All tests passed!"
    exit 0
  fi
}

run_tests
