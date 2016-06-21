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

set -o verbose
set -o errexit

HISTORY='gs://kubernetes-test-history/sq/history.txt'
GRAPH='gs://kubernetes-test-history/k8s-queue-health.svg'

POLLER='gs://kubernetes-test-history/sq/poller.py'
GRAPHER='gs://kubernetes-test-history/sq/graph.py'

install-deps() {
  for i in {1..5}; do
    sudo apt-get -y update || continue
    sudo apt-get -y install python-matplotlib python-requests || continue
    return 0
  done
  return 1
}

copy-to-old-location() {  # TODO(fejta): remove after pushing queue
  local old='gs://kubernetes-test-history/k8s-queue-health.png'
  while ps ax | grep -v grep | grep "${GRAPH}"; do
    sleep 60
    gsutil cp "${GRAPH}" "${old}" || true
  done
  echo "${GRAPH} no longer running, stopping copy to ${old}"
}

install-deps

gsutil cat "${POLLER}" | python - "${HISTORY}" 2>&1 | tee /var/log/poller.log &
gsutil cat "${GRAPHER}" | python - "${HISTORY}" "${GRAPH}" 2>&1 | tee /var/log/render.log &
copy-to-old-location
wait
