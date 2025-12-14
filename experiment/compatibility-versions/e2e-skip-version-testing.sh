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

skip_min_kubelet_versions() {
  local major_version=$1
  local minor_version_min=$2
  local minor_version_max=$3
  local start_minor

  if [[ minor_version_min -gt minor_version_max ]]; then
    return
  fi

  local skips=()
  for (( ver=minor_version_min; ver<=minor_version_max; ver++ )); do
    skips+=("\[MinimumKubeletVersion:${major_version}\.${ver}\]")
  done

  local skip_regex
  skip_regex=$(IFS='|'; echo "${skips[*]}")
  echo "skip kubelet tests with ${skip_regex}"

  if [[ -z "${skip_regex}" ]]; then
    return
  fi

  if [ -n "${SKIP:-}" ]; then
    export SKIP="${SKIP}|${skip_regex}"
  else
    export SKIP="${skip_regex}"
  fi
  echo "SKIP=${SKIP}"
}

run_skip_version_tests() {
  export PARALLEL=true
  ret=0
  while ! version_gte "$EMULATED_VERSION" "$CURRENT_VERSION"; do
    echo "Running e2e with compatibility version set to ${EMULATED_VERSION}"
    export PREV_RELEASE_BRANCH="release-${EMULATED_VERSION}"
    # Define the path within the temp directory for the cloned repo
    PREV_RELEASE_REPO_PATH="${TMP_DIR}/prev-release-k8s-${EMULATED_VERSION}"
    # Set value of emulated version 
    set_emulation_version "$EMULATED_VERSION" || ret=$?
    if [[ "$ret" -ne 0 ]]; then
      echo "Failed setting emulation version"
      return 1
    fi
    mkdir "${PREV_RELEASE_REPO_PATH}"
    pushd "${PREV_RELEASE_REPO_PATH}"
    download_release_version_bins ${EMULATED_VERSION} || ret=$?
    run_e2e_tests || ret=$?
    if [[ "$ret" -ne 0 ]]; then
      echo "Failed running skip version tests for emulated version $EMULATED_VERSION"
      return 1
    fi
    popd

    # Increment the minor version by 1
    major=$(echo "$EMULATED_VERSION" | cut -d. -f1)
    minor=$(echo "$EMULATED_VERSION" | cut -d. -f2)
    minor=$((minor + 1))
    export EMULATED_VERSION="${major}.${minor}"
  done
  # Test removal of emulated version entirely.
  CUR_REPO_PATH="${TMP_DIR}/cur-release-k8s-${EMULATED_VERSION}"
  mkdir ${CUR_REPO_PATH}
  pushd "${CUR_REPO_PATH}"
  delete_emulation_version || ret=$?
  download_current_version_bins ${LATEST_VERSION} || ret=$?
  run_e2e_tests || ret=$?
  popd
  return $ret
}

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
  skip_min_kubelet_versions $MAJOR_VERSION $((MINOR_VERSION - VERSION_DELTA + 1)) $MINOR_VERSION

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

  # Perform the upgrade.  Assume kind-upgrade.sh is in the same directory as this script.
  UPGRADE_SCRIPT="${UPGRADE_SCRIPT:-${PWD}/../test-infra/experiment/compatibility-versions/kind-upgrade.sh}"
  echo "Upgrading cluster with ${UPGRADE_SCRIPT}"

  upgrade_cluster_components || res=$?
  # If upgrade fails, we shouldn't run tests but should still clean up.
  if [[ "$res" -ne 0 ]]; then
    cleanup
    exit $res
  fi

  run_skip_version_tests || res=$?

  cleanup || res=$?
  exit $res
}

main