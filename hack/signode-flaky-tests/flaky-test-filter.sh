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

# Wrapper script for e2e jobs that injects flaky test skip or focus patterns.
#
# Usage in a job command:
#   command:
#     - runner.sh
#     - /home/prow/go/src/k8s.io/test-infra/hack/signode-flaky-tests/flaky-test-filter.sh
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
MODE="skip-regex"

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
        --mode=skip-regex|--mode=focus-regex)
            MODE="${1#*=}"
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

# Both modes need the generated regex. Focus jobs start with an empty focus filter,
# so they must fail closed if the tool cannot produce one.
# Build the flaky test tool from its own module.
SKIP_TOOL="$(mktemp /tmp/signode-flaky-tests.XXXXXX)"
trap 'rm -f "${SKIP_TOOL}"' EXIT

if ! (cd "${SCRIPT_DIR}" && go build -o "${SKIP_TOOL}" .); then
    if [[ "${MODE}" == "focus-regex" ]]; then
        echo "ERROR: failed to build signode-flaky-tests tool" >&2
        exit 1
    fi
    echo "WARNING: failed to build signode-flaky-tests tool, running without flaky skip" >&2
    rm -f "${SKIP_TOOL}"
    exec "$@"
fi

# Generate the requested regex.
TOOL_ARGS=(--config="${CONFIG}" --mode="${MODE}")
if [[ -n "${JOB_NAME}" ]]; then
    TOOL_ARGS+=(--job="${JOB_NAME}")
fi
if [[ -n "${SUITE}" ]]; then
    TOOL_ARGS+=(--suite="${SUITE}")
fi

FLAKY_REGEX=$("${SKIP_TOOL}" "${TOOL_ARGS[@]}")
rm -f "${SKIP_TOOL}"

if [[ -z "${FLAKY_REGEX}" ]]; then
    if [[ "${MODE}" == "focus-regex" ]]; then
        echo "ERROR: no flaky tests configured for focused run" >&2
        exit 1
    fi
    echo "signode-flaky-tests: no flaky tests configured, running as-is" >&2
    exec "$@"
fi

echo "signode-flaky-tests: applying flaky tests matching: ${FLAKY_REGEX}" >&2

if [[ "${MODE}" == "skip-regex" ]]; then
    REGEX_FLAGS=(--skip-regex --skip --ginkgo.skip)
else
    REGEX_FLAGS=(--focus-regex --focus --ginkgo.focus)
fi

shell_quote() {
    local escaped="${1//\'/\'\\\'\'}"
    printf "'%s'" "${escaped}"
}

ARGS=()
INJECTED=false
for arg in "$@"; do
    nested_test_args=false
    [[ "${arg}" == --test_args=* ]] && nested_test_args=true
    for regex_flag in "${REGEX_FLAGS[@]}"; do
        if [[ "${arg}" =~ (^|[[:space:]=])(${regex_flag//./[.]})=([^[:space:]]*) ]]; then
            match="${BASH_REMATCH[0]}"
            separator="${BASH_REMATCH[1]}"
            flag="${BASH_REMATCH[2]}"
            value="${BASH_REMATCH[3]}"
            value="${value#\"}"
            value="${value%\"}"
            value="${value#\'}"
            value="${value%\'}"
            regex="${value:+${value}|}${FLAKY_REGEX}"
            if [[ "${nested_test_args}" == "true" && "${flag}" != --ginkgo.* ]]; then
                regex="$(shell_quote "${regex}")"
            elif [[ "${nested_test_args}" == "true" ]]; then
                regex="${regex// /\\x20}"
            fi
            arg="${arg/"${match}"/"${separator}${flag}=${regex}"}"
            INJECTED=true
            break
        fi
    done
    ARGS+=("${arg}")
done

# Legacy test-e2e-node.sh jobs use SKIP instead of --skip-regex.
if [[ "${MODE}" == "skip-regex" && "${INJECTED}" == "false" && -n "${SKIP:-}" ]]; then
    export SKIP="${SKIP}|${FLAKY_REGEX}"
    INJECTED=true
fi

if [[ "${INJECTED}" == "false" ]]; then
    if [[ "${MODE}" == "focus-regex" ]]; then
        echo "ERROR: could not find a ${MODE} argument to inject flaky patterns" >&2
        exit 1
    fi
    echo "WARNING: could not find a ${MODE} argument to inject flaky patterns" >&2
fi

exec "${ARGS[@]}"
