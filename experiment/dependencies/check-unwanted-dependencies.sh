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

# Updates every dependency to latest, then reports which unwanted modules that
# drags in and where each came from. Fails on any new reference to a module in
# hack/unwanted-dependencies.json.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

WORKDIR="${ARTIFACTS:-${TMPDIR:-/tmp}}"
mkdir -p "${WORKDIR}"

export GOWORK=off
export GOFLAGS=-mod=mod
export PATH="${PATH}:${GOPATH:-${HOME}/go}/bin"
export GOCACHE="${GOCACHE:-"$(mktemp -d)/cache"}"

TEST_INFRA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# depstat gives the local dependency path for each unwanted module.
pushd "${WORKDIR}"
go install github.com/kubernetes-sigs/depstat@latest
popd

if ! command -v jq &> /dev/null; then
  apt update && apt -y install jq
fi

MAIN_MODULES="k8s.io/kubernetes$(ls staging/src/k8s.io | awk '{printf ",k8s.io/" $0}')"

# Baseline, to attribute each new reference to the bump that caused it.
go mod graph > "${WORKDIR}/before.graph"
cp go.mod "${WORKDIR}/before.go.mod"

# Update every dependency to latest, honouring spec.pinnedModules.
SKIP_PACKAGES=$(jq -r '.spec.pinnedModules | to_entries[] | .key' hack/unwanted-dependencies.json)
"${TEST_INFRA_DIR}/gomod_staleness.py" \
  --skip ${SKIP_PACKAGES} \
  --patch-output "${WORKDIR}/latest-go-mod-sum.patch" \
  --markdown-output "${WORKDIR}/differences.md"

# Rebuild vendor so the verifier can tell "referenced" from "vendored".
hack/update-vendor.sh

go mod graph > "${WORKDIR}/after.graph"
cp go.mod "${WORKDIR}/after.go.mod"

# Policy verdict. Captured, not fatal, so the report below still gets written.
VERIFIER_RC=0
go run k8s.io/kubernetes/cmd/dependencyverifier hack/unwanted-dependencies.json \
  > "${WORKDIR}/verifier.txt" 2>&1 || VERIFIER_RC=$?

# Where each new reference came from. Module proxy and GitHub API only.
ATTRIBUTION_RC=0
"${TEST_INFRA_DIR}/unwanted_deps.py" \
  --unwanted-json hack/unwanted-dependencies.json \
  --before-graph "${WORKDIR}/before.graph" \
  --after-graph "${WORKDIR}/after.graph" \
  --markdown-output "${WORKDIR}/unwanted-dependencies.md" \
  --text-output "${WORKDIR}/unwanted-dependencies.txt" \
  --json-output "${WORKDIR}/unwanted-dependencies.json" \
  --main-modules "${MAIN_MODULES}" || ATTRIBUTION_RC=$?

set +o xtrace

NEWLY_VENDORED=0
if grep -q "Unwanted modules are newly vendored" "${WORKDIR}/verifier.txt"; then
  NEWLY_VENDORED=1
fi

echo ""
echo "================ dependency policy summary ================"
echo "new references to unwanted modules : $([ "${ATTRIBUTION_RC}" -ne 0 ] && echo yes || echo no)"
echo "unwanted modules newly vendored    : $([ "${NEWLY_VENDORED}" -ne 0 ] && echo yes || echo no)"
echo "dependencyverifier exit code       : ${VERIFIER_RC}"
echo ""
echo "artifacts:"
echo "  unwanted-dependencies.txt  the table printed below"
echo "  unwanted-dependencies.md   same report as markdown, for pasting into an issue"
echo "  unwanted-dependencies.json same, machine readable"
echo "  verifier.txt               raw cmd/dependencyverifier output, for debugging"
echo "  differences.md             every dependency bumped by this run"
echo "==========================================================="
echo ""

# Report goes last so it is what you see first in the build log.
cat "${WORKDIR}/unwanted-dependencies.txt"

# Vendored means the code ships; referenced means the next bump trips the lint.
if [[ "${ATTRIBUTION_RC}" -ne 0 || "${NEWLY_VENDORED}" -ne 0 ]]; then
  echo "FAIL: updating dependencies introduces unwanted modules, see unwanted-dependencies.md"
  exit 1
fi

echo "PASS: no new unwanted dependencies introduced by updating everything to latest"
