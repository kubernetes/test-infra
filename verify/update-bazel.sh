#!/usr/bin/env bash
# Copyright 2016 The Kubernetes Authors.
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

TESTINFRA_ROOT=$(git rev-parse --show-toplevel)
TMP_GOPATH=$(mktemp -d)

"${TESTINFRA_ROOT}/verify/go_install_from_commit.sh" \
  github.com/kubernetes/repo-infra/kazel \
  e9d1a126ef355ff5d38e20612c889b07728225a4 \
  "${TMP_GOPATH}"

# The gazelle commit should match the rules_go commit in the WORKSPACE file.
"${TESTINFRA_ROOT}/verify/go_install_from_commit.sh" \
  github.com/bazelbuild/rules_go/go/tools/gazelle/gazelle \
  43a3bda3eb97e7bcd86f564a1e0a4b008d6c407c \
  "${TMP_GOPATH}"

"${TMP_GOPATH}/bin/gazelle" fix \
  -build_file_name=BUILD,BUILD.bazel \
  -external=vendored \
  -mode=fix \
  -repo_root="${TESTINFRA_ROOT}"

"${TMP_GOPATH}/bin/kazel" -root="${TESTINFRA_ROOT}"
