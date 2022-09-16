#!/usr/bin/env bash
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

set -o nounset
set -o errexit
set -o pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd $REPO_ROOT

echo "Ensuring go version."
source ./hack/build/setup-go.sh

# place to stick temp binaries
BINDIR="${REPO_ROOT}/_bin"
mkdir -p "${BINDIR}"

# TMP_REPO is used in make_temp_repo_copy
TMP_REPO="$(TMPDIR="${BINDIR}" mktemp -d "${BINDIR}/verify-deps.XXXXX")"

# exit trap cleanup for TMP_REPO
cleanup() {
  if [[ -n "${TMP_REPO}" ]]; then
    rm -rf "${TMP_REPO}"
  fi
}

# copies repo into a temp root saved to TMP_REPO
make_temp_repo_copy() {
  # we need to copy everything but bin (and the old _output) (which is .gitignore anyhow)
  find . \
    -mindepth 1 -maxdepth 1 \
    \( \
      -type d -path "./.git" -o \
      -type d -path "./_bin" -o \
      -type d -path "./_output" \
      -type d -path "./bazel-bin" \
      -type d -path "./bazel-out" \
      -type d -path "./bazel-test-infra" \
      -type d -path "./bazel-testlogs" \
    \) -prune -o \
    -exec bash -c 'cp -r "${0}" "${1}/${0}" >/dev/null 2>&1' {} "${TMP_REPO}" \;
}

main() {
  trap cleanup EXIT

  # copy repo root into tempdir under ./_bin
  make_temp_repo_copy

  # run generated code update script
  cd "${TMP_REPO}"
  REPO_ROOT="${TMP_REPO}" make update-go-deps

  # make sure the temp repo has no changes relative to the real repo
  diff=$(diff -Nupr \
          -x ".git" \
          -x "_bin" \
          -x "_output" \
          -x "bazel-bin" \
          -x "bazel-out" \
          -x "bazel-test-infra" \
          -x "bazel-testlogs" \
         "${REPO_ROOT}" "${TMP_REPO}" 2>/dev/null || true)
  if [[ -n "${diff}" ]]; then
    echo "unexpectedly dirty working directory after ${0}" >&2
    echo "" >&2
    echo "${diff}" >&2
    echo "" >&2
    echo "please run make update-go-deps" >&2
    exit 1
  fi
}

main
