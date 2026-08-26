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

# Wrapper script for kubetest2 jobs that appends flaky test skip patterns.
#
# Usage in a job command:
#   command:
#     - runner.sh
#     - /home/prow/go/src/k8s.io/test-infra/hack/signode-flaky-tests/skip-flaky.sh
#   args:
#     - --job-name=ci-cos-containerd-node-e2e-serial
#     - --suite=node_e2e
#     - --
#     - kubetest2
#     - noop
#     - --test=node
#     - --
#     - --skip-regex=\[Flaky\]|\[Slow\]
#     - ...remaining kubetest2 args...

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG="${SCRIPT_DIR}/flaky-tests.yaml"

JOB_NAME=""
SUITE=""

# Parse wrapper-specific flags before the -- separator.
while [[ $# -gt 0 ]]; do
    case "$1" in
        --job-name=*)
            JOB_NAME="${1#*=}"
            shift
            ;;
        --suite=*)
            SUITE="${1#*=}"
            shift
            ;;
        --)
            shift
            break
            ;;
        *)
            break
            ;;
    esac
done

# Build the flaky skip tool.
SKIP_TOOL="$(mktemp /tmp/signode-flaky-tests.XXXXXX)"
trap 'rm -f "${SKIP_TOOL}"' EXIT

if ! go build -o "${SKIP_TOOL}" "${SCRIPT_DIR}"; then
    echo "WARNING: failed to build signode-flaky-tests tool, running without flaky skip" >&2
    exec "$@"
fi

# Generate the skip regex.
TOOL_ARGS=(--config="${CONFIG}" --mode=skip-regex)
if [[ -n "${JOB_NAME}" ]]; then
    TOOL_ARGS+=(--job="${JOB_NAME}")
fi
if [[ -n "${SUITE}" ]]; then
    TOOL_ARGS+=(--suite="${SUITE}")
fi

FLAKY_REGEX=$("${SKIP_TOOL}" "${TOOL_ARGS[@]}")

if [[ -z "${FLAKY_REGEX}" ]]; then
    echo "signode-flaky-tests: no flaky tests configured, running as-is" >&2
    exec "$@"
fi

echo "signode-flaky-tests: skipping flaky tests matching: ${FLAKY_REGEX}" >&2

# Append the flaky regex to --skip-regex in the args.
ARGS=()
INJECTED=false
for arg in "$@"; do
    if [[ "${arg}" == --skip-regex=* ]]; then
        arg="${arg}|${FLAKY_REGEX}"
        INJECTED=true
    fi
    ARGS+=("${arg}")
done

# If no --skip-regex was found, try SKIP env var (used by test-e2e-node.sh).
if [[ "${INJECTED}" == "false" ]]; then
    if [[ -n "${SKIP:-}" ]]; then
        export SKIP="${SKIP}|${FLAKY_REGEX}"
        INJECTED=true
    fi
fi

if [[ "${INJECTED}" == "false" ]]; then
    echo "WARNING: could not find --skip-regex flag or SKIP env var to inject flaky patterns" >&2
fi

exec "${ARGS[@]}"
