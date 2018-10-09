#!/bin/bash
# Copyright 2018 The Kubernetes Authors.
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

# This script loads Prow configuration YAML and validates that the
# appropriate ProwJob is created from that configuration.

set -o errexit
set -o nounset
set -o pipefail

# We are told where the `mkpj` binary lives by Bazel
mkpj=$1
# We can either validate current behavior or update
# the recorded files
action=$2
if [[ "${action}" != "VALIDATE" && "${action}" != "GENERATE" ]]; then
    echo "[ERROR] Action must be either \`VALIDATE\` or \`GENERATE\`, not ${action}"
    exit 1
fi

output="$( mktemp -d )"

function cleanup() {
    returnCode="$?"
    rm -rf "${output}" || true
    exit "${returnCode}"
}
trap cleanup EXIT

function validate() {
    local expected=$1
    local actual=$2
    if ! difference="$( diff --ignore-matching-lines="name:" --ignore-matching-lines="startTime:" "${expected}" "${actual}" )"; then
        echo "[ERROR] Generated incorrect ProwJob YAML for config:"
        echo "${expected} vs ${actual}"
        echo "${difference}"
        exit 1
    fi
}

for prowJob in prow/test/data/*-prow-job.yaml; do
    prowJobName="$( basename "${prowJob}" "-prow-job.yaml" )"
    # we always pass all possible refs but `mkpj` will use
    # only those that are necessary for triggering the job
    "${mkpj}" --config-path prow/test/data/test-config.yaml \
              --job "${prowJobName}"                        \
              --base-ref "master" --base-sha "basesha"      \
              --pull-number "1" --pull-sha "pullsha" --pull-author "bob" > "${output}/${prowJobName}-prow-job.yaml"
    case "${action}" in
        "VALIDATE" )
            validate "${prowJob}" "${output}/${prowJobName}-prow-job.yaml" ;;
        "GENERATE" )
            mv "${output}/${prowJobName}-prow-job.yaml" "${prowJob}" ;;
    esac
done
