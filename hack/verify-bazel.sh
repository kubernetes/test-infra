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

cd $(git rev-parse --show-toplevel)

deprecated-verify() {
  OUTPUT_GOBIN="./_output/bin"
  GOBIN="${OUTPUT_GOBIN}" go install ./vendor/github.com/bazelbuild/bazel-gazelle/cmd/gazelle
  GOBIN="${OUTPUT_GOBIN}" go install ./vendor/k8s.io/repo-infra/kazel
  gazelle_diff=$("${OUTPUT_GOBIN}/gazelle" fix \
    -external=vendored \
    -mode=diff)

  kazel_diff=$("${OUTPUT_GOBIN}/kazel" \
    -dry-run \
    -print-diff\
    --cfg-path=./hack/.kazelcfg.json)

}

mkdir -p ./vendor
touch ./vendor/BUILD.bazel

if ! which bazel &> /dev/null; then
  echo "Bazel is the preferred way to build and test the test-infra repo." >&2
  echo "Please install bazel at https://bazel.build/ (future commits may require it)" >&2
  deprecated-verify
else
  gazelle_diff=$(bazel run //:gazelle-diff)
  kazel_diff=$(bazel run //:kazel-diff)
fi

if [[ -n "${gazelle_diff}" || -n "${kazel_diff}" ]]; then
  echo "${gazelle_diff}"
  echo "${kazel_diff}"
  echo
  echo "Run ./hack/update-bazel.sh"
  exit 1
fi

# Make sure there are no BUILD files outside vendor - we should only have
# BUILD.bazel files.
old_build_files=$(find . -name BUILD \( -type f -o -type l \) \
  -not -path './vendor/*' | sort)
if [[ -n "${old_build_files}" ]]; then
  echo "One or more BUILD files found in the tree:" >&2
  echo "${old_build_files}" >&2
  echo >&2
  echo "Only BUILD.bazel is allowed." >&2
  exit 1
fi
