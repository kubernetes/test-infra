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

# Usage: check-pr [ref]
#
# Ideally if check-pr passes then your golang PR will pass presubmit tests.

set -o nounset
set -o errexit
set -o pipefail

cd $(git rev-parse --show-toplevel)

dirs=()
tests=()
ref="${1:-HEAD}"
echo -n "Packages changed since $ref: "
for d in $(git diff --name-only "$ref" | xargs -n 1 dirname | sort -u); do
    if ! ls "./$d/"*.go &> /dev/null; then
        continue
    fi
    echo -n "$d "
    dirs+=("./$d")
    tests+=("//$d:all")
done

if [[ ${#dirs[@]} == 0 ]]; then
    echo NONE
    exit 0
fi
echo

failing=()
# step <name> <command ...> runs command and prints the output if it fails.
# if no <command> is specified, <name> is used as the command.
step() {
    name="$1"
    shift
    cmd="$@"

    echo -n "Running ${name}... "
    tmp="$(mktemp)"
    if [[ -z "${cmd}" ]]; then
        cmd="${name}"
    fi
    if ! ${cmd} &> "$tmp"; then
        echo FAIL:
        cat "$tmp"
        rm -f "$tmp"
        failing+=("${name}")
        return 0
    fi
    rm -f "$tmp"
    echo PASS
    return 0
}

step "//hack:verify-all" bazel test //hack:verify-all
step "bazel test" bazel test --build_tests_only "${tests[@]}"

if [[ "${#failing[@]}" != 0 ]]; then
    echo "FAILURE: ${#failing[@]} steps failed: ${failing[@]}"
    exit 1
fi
echo "SUCCESS"
