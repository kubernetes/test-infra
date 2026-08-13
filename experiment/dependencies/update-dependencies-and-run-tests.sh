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

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

# Default test mode
TEST_MODE="kind"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    --test-mode)
      TEST_MODE="$2"
      shift 2
      ;;
    *)
      echo "Unknown option: $1"
      echo "Usage: $0 [--test-mode <kind|unit>]"
      exit 1
      ;;
  esac
done

# Validate test mode
if [[ "${TEST_MODE}" != "kind" && "${TEST_MODE}" != "unit" ]]; then
  echo "Invalid test mode: ${TEST_MODE}. Must be either 'kind' or 'unit'."
  echo "Usage: $0 [--test-mode <kind|unit>]"
  exit 1
fi

# install kind
curl -sSL https://kind.sigs.k8s.io/dl/latest/linux-amd64.tgz | tar xvfz - -C "${PATH%%:*}/"

# install depstat, mdtohtml, and graphviz
export WORKDIR=${ARTIFACTS:-$TMPDIR}
export PATH=$PATH:$GOPATH/bin
export GOWORK=off
mkdir -p "${WORKDIR}"
pushd "$WORKDIR"
export GOCACHE="${GOCACHE:-"$(mktemp -d)/cache"}"
go install github.com/kubernetes-sigs/depstat@latest
go install github.com/sgaunet/mdtohtml@latest
popd

# install graphviz for dependency visualization
apt-get update -qq && apt-get install -y -qq graphviz > /dev/null 2>&1 || true

# needed by gomod_staleness.py
if ! command -v jq &> /dev/null; then
  apt update && apt -y install jq
fi

# Record the current commit for diff comparison
BEFORE_SHA=$(git rev-parse HEAD)
MAIN_MODULES="k8s.io/kubernetes$(ls staging/src/k8s.io | awk '{printf ",k8s.io/" $0}')"

# get the list of packages to skip from unwanted-dependencies.json
SKIP_PACKAGES=$(jq -r '.spec.pinnedModules | to_entries[] | .key' hack/unwanted-dependencies.json)

# update each dependency to the latest version, skipping the packages above
./../test-infra/experiment/dependencies/gomod_staleness.py \
  --skip ${SKIP_PACKAGES} \
  --patch-output "${WORKDIR}"/latest-go-mod-sum.patch \
  --markdown-output "${WORKDIR}"/differences.md
mdtohtml "${WORKDIR}"/differences.md "${WORKDIR}"/differences.html

# See if update-vendor still works
hack/update-vendor.sh

# Commit the dependency changes so depstat diff can checkout the base ref
git add -A
git commit -m "dependency updates" --allow-empty || true

# Generate dependency diff with visualization
echo ""
echo "=== Dependency Stats (before/after/delta) ==="
depstat diff "${BEFORE_SHA}" HEAD -m "${MAIN_MODULES}" --stats | tee "${WORKDIR}/stats.txt"
depstat diff "${BEFORE_SHA}" HEAD -m "${MAIN_MODULES}" --stats --json > "${WORKDIR}/stats.json"

echo ""
echo "=== Dependency Changes ==="
depstat diff "${BEFORE_SHA}" HEAD -m "${MAIN_MODULES}" -v --split-test-only --vendor --vendor-files | tee "${WORKDIR}/diff.txt"
depstat diff "${BEFORE_SHA}" HEAD -m "${MAIN_MODULES}" --split-test-only --vendor --vendor-files --json > "${WORKDIR}/diff.json"
depstat diff "${BEFORE_SHA}" HEAD -m "${MAIN_MODULES}" --dot > "${WORKDIR}/diff.dot"
dot -Tsvg "${WORKDIR}/diff.dot" -o "${WORKDIR}/diff.svg" || echo "Could not generate SVG"

# Show why each new dependency was added
ADDED_DEPS=$(jq -r '.added[]?' "${WORKDIR}/diff.json" 2>/dev/null || true)
if [ -n "${ADDED_DEPS}" ]; then
  echo ""
  echo "=== Why new dependencies are included ==="
  for dep in ${ADDED_DEPS}; do
    echo ""
    echo "--- ${dep} ---"
    depstat why "${dep}" -m "${MAIN_MODULES}" 2>/dev/null || echo "  (could not trace dependency path)"
  done | tee "${WORKDIR}/why-added.txt"
fi

# Explain vendor-only removals (removed from vendor, still in module graph)
VENDOR_ONLY_REMOVED=$(jq -r '.vendor.vendorOnlyRemovals[]?.path' "${WORKDIR}/diff.json" 2>/dev/null || true)
if [ -n "${VENDOR_ONLY_REMOVED}" ]; then
  echo ""
  echo "=== Why vendor-only removed modules are still in module graph ==="
  for dep in ${VENDOR_ONLY_REMOVED}; do
    echo ""
    echo "--- ${dep} ---"
    depstat why "${dep}" -m "${MAIN_MODULES}" 2>/dev/null || echo "  (could not trace dependency path)"
  done | tee "${WORKDIR}/why-vendor-only-removed.txt"
fi

echo ""
echo "=== High-signal dependency summary ==="
jq -r '[
  "Module graph: added=\(.added | length), removed=\(.removed | length), versionChanges=\(.versionChanges // [] | length)",
  "Non-test: added=\(.split.nonTestOnly.added // [] | length), removed=\(.split.nonTestOnly.removed // [] | length), versionChanges=\(.split.nonTestOnly.versionChanges // [] | length)",
  "Test-only: added=\(.split.testOnly.added // [] | length), removed=\(.split.testOnly.removed // [] | length), versionChanges=\(.split.testOnly.versionChanges // [] | length)",
  "Vendor: added=\(.vendor.added // [] | length), removed=\(.vendor.removed // [] | length), versionChanges=\(.vendor.versionChanges // [] | length), vendorOnlyRemovals=\(.vendor.vendorOnlyRemovals // [] | length)",
  "Vendor files: added=\(.vendor.filesAdded // [] | length), deleted=\(.vendor.filesDeleted // [] | length)"
] | .[]' "${WORKDIR}/diff.json"

# Do not worry if this fails, it is bound to fail
hack/lint-dependencies.sh || true

# ensure that all our code will compile
hack/verify-typecheck.sh

# run tests based on the selected mode
if [[ "${TEST_MODE}" == "kind" ]]; then
  # run kind based e2e tests
  e2e-k8s.sh
else
  # run unit tests
  make test
fi
