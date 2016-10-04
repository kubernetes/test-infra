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

# Download the latest version of kops, then run the e2e tests using e2e.sh.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

readonly KOPS_LATEST=${KOPS_LATEST:-"latest-ci.txt"}
readonly LATEST_URL="https://storage.googleapis.com/kops-ci/bin/${KOPS_LATEST}"
readonly KOPS_URL=$(curl -fsS --retry 3 "${LATEST_URL}")
if [[ -z "${KOPS_URL}" ]]; then
  echo "Can't fetch kops URL" >&2
  exit 1
fi

curl -fsS --retry 3 -o "${WORKSPACE}/kops" "${KOPS_URL}/linux/amd64/kops"
chmod +x "${WORKSPACE}/kops"
export NODEUP_URL="${KOPS_URL}/linux/amd64/nodeup"

# Get kubectl on the path (works after e2e-runner.sh:unpack_binaries)
export PATH="${PATH}:/workspace/kubernetes/platforms/linux/amd64"

$(dirname "${BASH_SOURCE}")/e2e-runner.sh
