#!/usr/bin/env bash
# Copyright 2017 The Kubernetes Authors.
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
set -o xtrace

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd "${REPO_ROOT}"

migrate() {
  if [[ -z "${2}" ]]; then
    exit 1
  fi
  "${REPO_ROOT}/_bin/migratestatus" \
    --dry-run=false \
    --org=kubernetes \
    --repo=kubernetes \
    --tokenfile ~/github-token \
    --retire="${1}" \
    --dest="${2}"
}

./hack/make-rules/go-run/arbitrary.sh build -o "${REPO_ROOT}/_bin/migratestatus" ./maintenance/migratestatus || exit 1

#migrate "Bazel test" pull-test-infra-bazel
#migrate "Gubernator tests" pull-test-infra-gubernator
#migrate "verify-bazel" pull-test-infra-verify-bazel

migrate "Jenkins GCE etcd3 e2e" pull-kubernetes-e2e-gce-etcd3
migrate "Jenkins kops AWS e2e" pull-kubernetes-e2e-kops-aws
migrate "Jenkins unit/integration" pull-kubernetes-unit
migrate "Jenkins verification" pull-kubernetes-verify
migrate "Jenkins GCE Node e2e" pull-kubernetes-node-e2e
