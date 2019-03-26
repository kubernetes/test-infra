#!/usr/bin/env bash
# Copyright 2019 The Kubernetes Authors.
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

cd "$(git rev-parse --show-toplevel)"

export GOPATH="${GOPATH:-${HOME}/go}"
export PATH="${GOPATH}/bin:${PATH}"

ensure-in-gopath() {
  if [[ "${PWD}" != "${GOPATH}/src/k8s.io/test-infra" ]]; then
    echo "Sadly, $(basename "${0}") must run inside GOPATH=${GOPATH}, not ${PWD}" >&2
    exit 1
  fi
}

codegen-init() {
  echo "Ensuring generators exist..." >&2
  go install ./vendor/k8s.io/code-generator/cmd/{deepcopy,defaulter}-gen
}

gen-deepcopy() {
  echo "Generating DeepCopy() methods..." >&2
  deepcopy-gen \
    --go-header-file hack/boilerplate/boilerplate.generated.go.txt \
    --input-dirs k8s.io/test-infra/experiment/bazinga/pkg/config \
    --output-file-base zz_generated.deepcopy
}

gen-defaulter() {
  echo "Generating Defaults..." >&2
  defaulter-gen \
    --go-header-file hack/boilerplate/boilerplate.generated.go.txt \
    --input-dirs k8s.io/test-infra/experiment/bazinga/pkg/config \
    --output-file-base zz_generated.default
}

codegen-init
gen-deepcopy
gen-defaulter
