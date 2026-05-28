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

# Terminates Lambda Cloud instances tagged managed-by=prow-k8s-ci
# whose expires-at is in the past. Requires LAMBDA_API_KEY_FILE
# (supplied by preset-lambda-credential); honors ARTIFACTS for logs.

set -o errexit
set -o nounset
set -o pipefail

GOPROXY=direct go install github.com/dims/lambdactl@latest

NOW=$(date +%s)
LOG_DIR="${ARTIFACTS:-/tmp}/lambda-janitor"
mkdir -p "${LOG_DIR}"
INSTANCES_FILE="${LOG_DIR}/instances.json"

echo "Lambda janitor pass at epoch ${NOW}"

lambdactl --json instances > "${INSTANCES_FILE}"
TOTAL=$(jq 'length' "${INSTANCES_FILE}")
echo "Total instances on account: ${TOTAL}"

echo "--- Managed instances (managed-by=prow-k8s-ci) ---"
jq -r '
  .[] |
  select(.tags[]? | .key=="managed-by" and .value=="prow-k8s-ci") |
  ((.tags | map({(.key): .value}) | add) // {}) as $t |
  "  \(.id)  status=\(.status)  expires-at=\($t["expires-at"] // "<none>")  job=\($t["prow-job"] // "<none>")  build=\($t["build-id"] // "<none>")"
' "${INSTANCES_FILE}"

# Skip and report managed instances whose expires-at is not a
# non-negative integer; one bad tag must not block the rest of the pass.
INVALID=$(jq '
  [
    .[] |
    select(.tags[]? | .key=="managed-by" and .value=="prow-k8s-ci") |
    select(.status != "terminating" and .status != "terminated") |
    ((.tags | map({(.key): .value}) | add) // {}) as $t |
    select($t["expires-at"] != null) |
    select($t["expires-at"] | test("^[0-9]+$") | not) |
    {id, expires_at: $t["expires-at"], job: $t["prow-job"]}
  ]
' "${INSTANCES_FILE}")
INVALID_COUNT=$(jq 'length' <<<"${INVALID}")
if [[ "${INVALID_COUNT}" != "0" ]]; then
  echo "WARNING: ${INVALID_COUNT} managed instance(s) have non-numeric expires-at; skipping:"
  jq -r '.[] | "  \(.id)  expires-at=\(.expires_at|@json)  job=\(.job // "<none>")"' <<<"${INVALID}"
fi

# Select managed, non-terminating instances whose expires-at is a
# non-negative integer in the past. The regex gate matches the
# invalid-pass above, so tonumber here is unconditionally safe.
EXPIRED=$(jq --arg now "${NOW}" '
  [
    .[] |
    select(.tags[]? | .key=="managed-by" and .value=="prow-k8s-ci") |
    select(.status != "terminating" and .status != "terminated") |
    ((.tags | map({(.key): .value}) | add) // {}) as $t |
    select($t["expires-at"] != null) |
    select($t["expires-at"] | test("^[0-9]+$")) |
    select(($t["expires-at"] | tonumber) < ($now | tonumber))
  ]
' "${INSTANCES_FILE}")
EXPIRED_COUNT=$(jq 'length' <<<"${EXPIRED}")
echo "--- Expired instances to terminate: ${EXPIRED_COUNT} ---"

if [[ "${EXPIRED_COUNT}" == "0" ]]; then
  echo "No expired instances; exiting."
  exit 0
fi

FAILS=0
while read -r id; do
  echo "Terminating ${id}..."
  if ! lambdactl stop "${id}" --yes; then
    echo "  FAILED to terminate ${id}"
    FAILS=$((FAILS + 1))
  fi
done < <(jq -r '.[].id' <<<"${EXPIRED}")

echo "Pass complete. terminated=$((EXPIRED_COUNT - FAILS)) failures=${FAILS}"

if [[ "${FAILS}" -gt 0 ]]; then
  exit 1
fi
