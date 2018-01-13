#!/bin/bash
# Copyright 2015 The Kubernetes Authors.
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

# Run e2e tests using environment variables exported in e2e.sh.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

export PS4='+(${BASH_SOURCE}:${LINENO}): ${FUNCNAME[0]:+${FUNCNAME[0]}(): }'

echo 'WARNING: e2e-runner.sh is deprecated! This will error on August 2nd' >&2
echo 'Call kubetest directly' >&2
echo 'More info: https://github.com/kubernetes/test-infra/issues/2829' >&2

if [[ "$(date +%s)" -gt "$(date --date='August 1 2017' +%s)" ]]; then
  echo 'e2e-runner.sh expired on August 1st, failing'
  exit 1
elif [[ "$(date +%s)" -gt "$(date --date='July 25 2017' +%s)" ]]; then
  # Delay five minutes and spam the logs
  for i in {1..100}; do
    echo "e2e-runner.sh will expire in a week, please update job (notice ${i})" >&2
    sleep 3
  done
fi

kubetest "${@}"
