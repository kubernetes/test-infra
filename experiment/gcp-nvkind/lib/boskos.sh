#!/usr/bin/env bash
# Copyright The Kubernetes Authors.
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

# boskos.sh -- acquire / heartbeat / release a Boskos resource.
#
# Required env: BOSKOS_HOST, BOSKOS_RESOURCE_TYPE, JOB_NAME.
# On success exports: BOSKOS_RESOURCE_JSON, BOSKOS_PROJECT.

set -o errexit
set -o nounset
set -o pipefail

# sigs.k8s.io/boskos has no tags; pin a commit SHA. Bump with a reviewable diff.
: "${BOSKOSCTL_VERSION:=c2c6f437ca1db28863fb1cbaaa74b8ea4784483f}"

boskos::install_cli() {
  if command -v boskosctl >/dev/null 2>&1; then return 0; fi
  echo "Installing boskosctl@${BOSKOSCTL_VERSION}..."
  GO111MODULE=on go install "sigs.k8s.io/boskos/cmd/boskosctl@${BOSKOSCTL_VERSION}"
  export PATH="${PATH}:$(go env GOPATH)/bin"
}

boskos::acquire() {
  : "${BOSKOS_HOST:?BOSKOS_HOST must be set}"
  : "${BOSKOS_RESOURCE_TYPE:?BOSKOS_RESOURCE_TYPE must be set}"
  : "${JOB_NAME:?JOB_NAME must be set}"

  boskos::install_cli

  echo "Acquiring a ${BOSKOS_RESOURCE_TYPE} from ${BOSKOS_HOST}..."
  BOSKOS_RESOURCE_JSON=$(boskosctl \
    --server-url "${BOSKOS_HOST}" \
    --owner-name "${JOB_NAME}" \
    acquire \
    --type "${BOSKOS_RESOURCE_TYPE}" \
    --state free \
    --target-state busy \
    --timeout 30m)
  BOSKOS_PROJECT=$(jq -r .name <<<"${BOSKOS_RESOURCE_JSON}")
  export BOSKOS_RESOURCE_JSON BOSKOS_PROJECT
  echo "Acquired project: ${BOSKOS_PROJECT}"
}

boskos::heartbeat_start() {
  : "${BOSKOS_RESOURCE_JSON:?call boskos::acquire first}"
  boskosctl \
    --server-url "${BOSKOS_HOST}" \
    --owner-name "${JOB_NAME}" \
    heartbeat \
    --resource "${BOSKOS_RESOURCE_JSON}" \
    --period 30s &
  BOSKOS_HEARTBEAT_PID=$!
  export BOSKOS_HEARTBEAT_PID
  echo "Heartbeat started (pid ${BOSKOS_HEARTBEAT_PID})"
}

boskos::release() {
  if [ -n "${BOSKOS_HEARTBEAT_PID:-}" ]; then
    kill "${BOSKOS_HEARTBEAT_PID}" 2>/dev/null || true
    wait "${BOSKOS_HEARTBEAT_PID}" 2>/dev/null || true
  fi
  if [ -n "${BOSKOS_PROJECT:-}" ]; then
    echo "Releasing project ${BOSKOS_PROJECT} back to the pool (dirty)..."
    boskosctl \
      --server-url "${BOSKOS_HOST}" \
      --owner-name "${JOB_NAME}" \
      release \
      --name "${BOSKOS_PROJECT}" \
      --target-state dirty || true
  fi
}
