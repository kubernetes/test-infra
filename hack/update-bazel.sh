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

# https://github.com/kubernetes/test-infra/issues/5699#issuecomment-348350792
cd $(git rev-parse --show-toplevel)

# Old way of running gazelle and kazel by first go installing them
deprecated-update() {
  OUTPUT_GOBIN="./_output/bin"
  GOBIN="${OUTPUT_GOBIN}" go install ./vendor/github.com/bazelbuild/bazel-gazelle/cmd/gazelle
  GOBIN="${OUTPUT_GOBIN}" go install ./vendor/github.com/kubernetes/repo-infra/kazel

  "${OUTPUT_GOBIN}/gazelle" fix --external=vendored --mode=fix
  "${OUTPUT_GOBIN}/kazel" --cfg-path=./hack/.kazelcfg.json
}

# Ensure ./vendor/BUILD.bazel exists
mkdir -p ./vendor
touch "./vendor/BUILD.bazel"

if ! which bazel &> /dev/null; then
  echo "Bazel is the preferred way to build and test the test-infra repo." >&2
  echo "Please install bazel at https://bazel.build/ (future commits may require it)" >&2
  deprecated-update
  exit 0
fi
bazel run //:gazelle
bazel run //:kazel
