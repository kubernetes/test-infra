#!/usr/bin/env bash
# Copyright 2024 The Kubernetes Authors.
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

# hack script for running a kind e2e
# must be run with a kubernetes checkout in $PWD (IE from the checkout)
# Usage: SKIP="ginkgo skip regex" FOCUS="ginkgo focus regex" kind-e2e.sh

set -o errexit -o nounset -o xtrace

# Settings:
# SKIP: ginkgo skip regex
# FOCUS: ginkgo focus regex
# LABEL_FILTER: ginkgo label query for selecting tests (see "Spec Labels" in https://onsi.github.io/ginkgo/#filtering-specs)
#
# The default is to focus on conformance tests. Serial tests get skipped when
# parallel testing is enabled. Using LABEL_FILTER instead of combining SKIP and
# FOCUS is recommended (more expressive, easier to read than regexp).
#
# FEATURE_GATES:
#          JSON or YAML encoding of a string/bool map: {"FeatureGateA": true, "FeatureGateB": false}
#          Enables or disables feature gates in the entire cluster.
# RUNTIME_CONFIG:
#          JSON or YAML encoding of a string/string (!) map: {"apia.example.com/v1alpha1": "true", "apib.example.com/v1beta1": "false"}
#          Enables API groups in the apiserver via --runtime-config.

COMMON_SCRIPT="${COMMON_SCRIPT:-${PWD}/../test-infra/experiment/compatibility-versions/common.sh}"
source "${COMMON_SCRIPT}"

# setup signal handlers
# shellcheck disable=SC2317 # this is not unreachable code
signal_handler() {
  if [ -n "${GINKGO_PID:-}" ]; then
    kill -TERM "$GINKGO_PID" || true
  fi
  cleanup
}
trap signal_handler INT TERM

main() {
  # create temp dir and setup cleanup
  TMP_DIR=$(mktemp -d -p /tmp kind-e2e-XXXXXX)
  echo "Created temporary directory: ${TMP_DIR}"

  # ensure artifacts (results) directory exists when not in CI
  export ARTIFACTS="${ARTIFACTS:-${PWD}/_artifacts}"
  mkdir -p "${ARTIFACTS}"

  export VERSION_DELTA=${VERSION_DELTA:-1}

  WORKSPACE_STATUS=$(./hack/print-workspace-status.sh)
  GIT_VERSION=$(echo "$WORKSPACE_STATUS" | awk '/^gitVersion / {print $2}')
  # Check if gitVersion contains alpha.0 and increment VERSION_DELTA if needed
  # If the current version is alpha.0, it means the previous *stable* or developed
  # branch is actually n-2 relative to the current minor number for compatibility purposes.
  if [[ "${GIT_VERSION}" == *alpha.0* ]]; then
    echo "Detected alpha.0 in gitVersion (${GIT_VERSION}), treating as still the previous minor version."
    VERSION_DELTA=$((VERSION_DELTA + 1))
    echo "Adjusted VERSION_DELTA: ${VERSION_DELTA}"
  fi

  MAJOR_VERSION=$(echo "$WORKSPACE_STATUS" | awk '/^STABLE_BUILD_MAJOR_VERSION / {print $2}')
  MINOR_VERSION=$(echo "$WORKSPACE_STATUS" | awk '/^STABLE_BUILD_MINOR_VERSION / {split($2, minor, "+"); print minor[1]}')
  export CURRENT_VERSION="${MAJOR_VERSION}.${MINOR_VERSION}"
  export PREV_VERSION="${MAJOR_VERSION}.$((MINOR_VERSION - VERSION_DELTA))"
  export EMULATED_VERSION="${PREV_VERSION}"

  # export the KUBECONFIG to a unique path for testing
  KUBECONFIG="${HOME}/.kube/kind-test-config"
  export KUBECONFIG
  echo "exported KUBECONFIG=${KUBECONFIG}"

  # debug kind version
  kind version

  # in CI attempt to release some memory after building
  if [ -n "${KUBETEST_IN_DOCKER:-}" ]; then
    sync || true
    echo 1 > /proc/sys/vm/drop_caches || true
  fi

  res=0
  create_cluster || res=$?

  # first step: upgrade binary version while keeping emulated version the same as previous version
  # Perform the upgrade.  Assume kind-upgrade.sh is in the same directory as this script.
  UPGRADE_SCRIPT="${UPGRADE_SCRIPT:-${PWD}/../test-infra/experiment/compatibility-versions/kind-upgrade.sh}"
  echo "Upgrading cluster with ${UPGRADE_SCRIPT}"

  upgrade_cluster_components || res=$?
  # If upgrade fails, we shouldn't run tests but should still clean up.
  if [[ "$res" -ne 0 ]]; then
    cleanup
    exit $res
  fi

  # after first step of upgrading binary version without bumping emulated version, verify all tests from the previous version pass

  # Clone the previous versions Kubernetes release branch
  # TODO(aaron-prindle) extend the branches to test from n-1 -> n-1..3 as more k8s releases are done that support compatibility versions
  export PREV_RELEASE_BRANCH="release-${PREV_VERSION}"
  # Define the path within the temp directory for the cloned repo
  PREV_RELEASE_REPO_PATH="${TMP_DIR}/prev-release-k8s"
  # enter the cloned prev repo branch (in temp) and run tests
  mkdir "${PREV_RELEASE_REPO_PATH}"
  pushd "${PREV_RELEASE_REPO_PATH}"
  download_release_version_bins ${EMULATED_VERSION} || res=$?
  run_e2e_tests || res=$?
  # debug kubectl version
  kubectl version
  # remove "${PWD}/_output/bin" from PATH
  export PATH="${PATH//${PWD}\/_output\/bin:}"
  popd

  # second step: bump emulated version
  EMULATED_VERSION_UPGRADE_SCRIPT="${EMULATED_VERSION_UPGRADE_SCRIPT:-${PWD}/../test-infra/experiment/compatibility-versions/emulated-version-upgrade.sh}"
  echo "Upgrading cluster with ${EMULATED_VERSION_UPGRADE_SCRIPT}"
  "${EMULATED_VERSION_UPGRADE_SCRIPT}" | tee "${ARTIFACTS}/emulated-upgrade-output.txt"

  # verify all tests from the current version pass after upgrade is complete
  # debug kubectl version
  kubectl version
  # run tests at head
  CUR_RELEASE_REPO_PATH="${TMP_DIR}/cur-release-k8s"
  mkdir "${CUR_RELEASE_REPO_PATH}"
  pushd "${CUR_RELEASE_REPO_PATH}"
  download_current_version_bins ${LATEST_VERSION} || ret=$?
  run_e2e_tests || res=$?
  popd
  cleanup || res=$?
  exit $res
}

main
