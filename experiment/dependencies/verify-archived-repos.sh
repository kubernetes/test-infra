#!/usr/bin/env bash

# Copyright 2025 The Kubernetes Authors.
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

# Verify all archived Go dependencies are tracked in hack/unwanted-dependencies.json.
# Usage: ./verify.sh [path/to/kubernetes]

set -euo pipefail

K8S_DIR="${1:-.}"
K8S_DIR="$(cd "$K8S_DIR" && pwd)"
UNWANTED_FILE="${K8S_DIR}/hack/unwanted-dependencies.json"
[[ -f "$UNWANTED_FILE" ]] || { echo "ERROR: ${UNWANTED_FILE} not found."; exit 1; }

TOKEN_PATH="/etc/github/token"
DEPSTAT_TOKEN_ARGS=()
if [[ -f "$TOKEN_PATH" ]]; then
  DEPSTAT_TOKEN_ARGS=(--github-token-path "$TOKEN_PATH")
elif [[ -z "${GITHUB_TOKEN:-}" ]]; then
  echo "ERROR: GitHub token required. Place at ${TOKEN_PATH} or set GITHUB_TOKEN."; exit 1
fi

go install github.com/kubernetes-sigs/depstat@latest
JSON_OUTPUT=$(depstat archived --json --dir="$K8S_DIR" "${DEPSTAT_TOKEN_ARGS[@]}")

FAILURES=0
while IFS= read -r module; do
  reason=$(jq -r --arg mod "$module" '.spec.unwantedModules[$mod] // empty' "$UNWANTED_FILE")
  if [[ -z "$reason" ]]; then
    repo_url=$(echo "$JSON_OUTPUT" | jq -r --arg mod "$module" '.archived[] | select(.module == $mod) | .repoUrl' | head -1)
    version=$(echo "$JSON_OUTPUT" | jq -r --arg mod "$module" '.archived[] | select(.module == $mod) | .version' | head -1)
    echo "FAIL: ${module} (${version}) archived at ${repo_url} — not in unwanted-dependencies.json"
    FAILURES=$((FAILURES + 1))
  else
    echo "OK:   ${module} (${reason})"
  fi
done < <(echo "$JSON_OUTPUT" | jq -r '.archived[].module')

if [[ "$FAILURES" -gt 0 ]]; then
  echo "FAILED: ${FAILURES} archived dep(s) not tracked in unwanted-dependencies.json"
  exit 1
fi
echo "PASSED: All archived dependencies are tracked."
