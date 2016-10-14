#!/bin/bash
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
set -o xtrace

readonly testinfra="$(dirname "${0}")/.."

# TODO(fejta): how to handle this situation??
# verify-godeps compares against upstream, but remote/master might be stale
# if .git was retained between runs. Update it explicitly here.
git fetch remote master:refs/remotes/remote/master
# similarly, verify-munge-docs compares against the latest release branch.
git fetch remote release-1.4:remote/release-1.4
export KUBE_VERIFY_GIT_BRANCH="${ghprbTargetBranch}"
export KUBE_TEST_SCRIPT="./hack/jenkins/verify-dockerized.sh"
./hack/jenkins/gotest-dockerized.sh
