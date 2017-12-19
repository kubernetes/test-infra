#!/bin/bash

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

function port-forward {
  PORT=${PORT:-9999}
  PODNAME=$(kubectl get endpoints submit-queue-status -o template --template='{{index . "subsets" 0 "addresses" 0 "targetRef" "name"}}')
  echo "Admin interface will serve on port ${PORT}. Press ctrl-c to stop serving."
  kubectl port-forward "${PODNAME}" "${PORT}:9999"
}

function wait-for-port {
  PORT=${PORT:-9999}
  for (( i=0; i < 20; i++ )); do
    sleep .25
    if curl -s2 "localhost:${PORT}/" 2>&1 > /dev/null; then
      echo "Admin port opened"
      return 0
    fi
  done
  echo "Admin port never became responsive."
  return 1
}
