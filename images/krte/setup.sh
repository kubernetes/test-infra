#!/usr/bin/env bash
# Copyright 2020 The Kubernetes Authors.
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

# Checks to see if kubetest2 exists in the test image, if not go gets it
# at $KUBETEST2_VERSION (if specified) or latest

set -o errexit
set -o nounset
set -o pipefail

# shellcheck disable=SC2230
if ! which kubetest2 > /dev/null; then
  (cd && GO111MODULE=on go get sigs.k8s.io/kubetest2/...@"${KUBETEST2_VERSION:-latest}")
fi

wrapper.sh "$@"
