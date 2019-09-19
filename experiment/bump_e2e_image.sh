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

# Usage: bump_e2e_image.sh

set -o errexit
set -o nounset
set -o pipefail

TREE="$(git rev-parse --show-toplevel)"

bazel run //experiment/image-bumper -- --image-regex gcr.io/k8s-testimages/kubekins-e2e "${TREE}/experiment/generate_tests.py" "${TREE}/experiment/test_config.yaml" "${TREE}/config/prow/config.yaml"
find "${TREE}/config/jobs/" . -name "*.yaml" | xargs bazel run //experiment/image-bumper -- --image-regex gcr.io/k8s-testimages/kubekins-e2e

bazel run //experiment:generate_tests -- \
  "--yaml-config-path=${TREE}/experiment/test_config.yaml" \
  "--output-dir=${TREE}/config/jobs/kubernetes/generated/"

git commit -am "Bump gcr.io/k8s-testimages/kubekins-e2e (using generate_tests and manual)"
