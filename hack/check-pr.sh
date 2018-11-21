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

# step <name> <command ...> runs command and prints the output if it fails.
step() {
    echo -n "Running $1... "
    shift
    tmp="$(mktemp)"
    if ! "$@" &> "$tmp"; then
        echo FAIL:
        cat "$tmp"
        rm -f "$tmp"
        return 1
    fi
    rm -f "$tmp"
    echo PASS
    return 0
}

failing=()
step hack/verify-bazel.sh hack/verify-bazel.sh || failing+=("bazel")
step hack/verify-gofmt.sh hack/verify-gofmt.sh || failing+=("gofmt")
step //:golint bazel run //:golint -- "${dirs[@]}" || failing+=("golint")
step //:govet bazel run //:govet -- "${dirs[@]}" || failing+=("govet")
step "bazel test" bazel test --build_tests_only "${tests[@]}" || failing+=("bazel test")
step hack/verify_boilerplate.py hack/verify_boilerplate.py || failing+=("boilerplate")

if [[ "${#failing[@]}" != 0 ]]; then
    echo "FAILURE: ${#failing[@]} steps failed: ${failing[@]}"
    exit 1
fi
echo "SUCCESS"
