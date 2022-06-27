#!/usr/bin/env bash
# Copyright 2021 The Kubernetes Authors.
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

SCRIPT_ROOT="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_ROOT}"/lib.sh

# shellcheck disable=SC1091
source "${REPO_ROOT}/hack/build/setup-go.sh"

function usage() {
  >&2 cat <<EOF
Run Prow's integration tests.

Usage: $0 [options]

Examples:
  # Bring up the KIND cluster and Prow components, but only run the
  # "TestClonerefs/postsubmit" test.
  $0 -run=Clonerefs/post

  # Only run the "TestClonerefs/postsubmit" test, with increased verbosity.
  $0 -verbose -no-setup -run=Clonerefs/post

  # Recompile and redeploy the Prow components that use the "fakegitserver" and
  # "fakegerritserver" images, then only run the "TestClonerefs/postsubmit"
  # test, but also
  $0 -verbose -build=fakegitserver,fakegerritserver -run=Clonerefs/post

  # Recompile "deck" image, redeploy "deck" and "deck-tenanted" Prow components,
  # then only run the "TestDeck" tests. The test knows that "deck" and
  # "deck-tenanted" components both depend on the "deck" image in lib.sh (grep
  # for PROW_IMAGES_TO_COMPONENTS).
  $0 -verbose -build=deck -run=Clonerefs/post

  # Recompile all Prow components, redeploy them, and then only run the
  # "TestClonerefs/postsubmit" test.
  $0 -verbose -no-setup-kind-cluster -run=Clonerefs/post

  # Before running the "TestClonerefs/postsubmit" test, delete all ProwJob
  # Custom Resources and test pods from test-pods namespace.
  $0 -verbose -no-setup-kind-cluster -run=Clonerefs/post -clear=ALL

Options:
    -no-setup:
        Skip setup of the KIND cluster and Prow installation. That is, only run
        gotestsum. This is useful if you already have the cluster and components
        set up, and only want to run some tests without setting up the cluster
        or recompiling Prow images.

    -no-setup-kind-cluster:
        Skip setup of the KIND cluster, but still (re-)install Prow to the
        cluster. Flag "-build=..." implies this flag. This is useful if you want
        to skip KIND setup. Most of the time, you will want to use this flag
        when rerunning tests after initially setting up the cluster (because
        most of the time your changes will not impact the KIND cluster itself).

    -build='':
        Build only the comma-separated list of Prow components with
        "${REPO_ROOT}"/hack/prowimagebuilder. Useful when developing a fake
        service that needs frequent recompilation. The images are a
        comma-separated string. Also results in only redeploying certain entries
        in PROW_COMPONENTS, by way of PROW_IMAGES_TO_COMPONENTS in lib.sh.

        The value "ALL" for this falg is an alias for all images (PROW_IMAGES in
        lib.sh).

        By default, "-build=ALL" is assumed, so that users do not have to
        provide any arguments to this script to run all tests.

        Implies -no-setup-kind-cluster.

    -clear='':
        Delete the comma-separated list of Kubernetes resources from the KIND
        cluster before running the test. Possible values: "ALL", "prowjobs",
        "test-pods". ALL is an alias for prowjobs and test-pods.

        This makes it easier to see the exact ProwJob Custom Resource ("watch
        kubectl get prowjobs.prow.k8s.io") or associated test pod ("watch
        kubectl get pods -n test-pods") that is created by the test being run.

    -run='':
        Run only those tests that match the given pattern. The format is
        "TestName/testcasename". E.g., "TestClonerefs/postsubmit" will only run
        1 test. Due to fuzzy matching, "Clonerefs/post" is equivalent.

    -save-logs='':
        Export all cluster logs to the given directory (directory will be
        created if it doesn't exist).

    -teardown:
        Delete the KIND cluster and also the local Docker registry used by the
        cluster.

    -verbose:
        Make tests run more verbosely.

    -help:
        Display this help message.
EOF
}

function main() {
  declare -a tests_to_run
  declare -a setup_args
  declare -a clear_args
  declare -a teardown_args
  setup_args=(-setup-kind-cluster -setup-prow-components -build=ALL)
  local summary_format
  summary_format=pkgname
  local setup_kind_cluster
  local setup_prow_components
  local build_images
  local resource
  local resources_val
  local fakepubsub_node_port
  setup_kind_cluster=0
  setup_prow_components=0

  for arg in "$@"; do
    case "${arg}" in
      -no-setup)
        unset 'setup_args[0]'
        unset 'setup_args[1]'
        unset 'setup_args[2]'
        ;;
      -no-setup-kind-cluster)
        unset 'setup_args[0]'
        ;;
      -build=*)
        # Imply -no-setup-kind-cluster.
        unset 'setup_args[0]'
        # Because we specified a "-build=..." flag explicitly, drop the default
        # "-build=ALL" option.
        unset 'setup_args[2]'
        setup_args+=("${arg}")
        ;;
      -clear=*)
        resources_val="${arg#-clear=}"
        for resource in ${resources_val//,/ }; do
          case "${resource}" in
            ALL)
              clear_args=(-prowjobs -test-pods)
            ;;
            prowjobs|test-pods)
              clear_args+=("${resource}")
            ;;
            *)
              echo >&2 "unrecognized argument to -clear: ${resource}"
              return 1
            ;;
          esac
        done
        ;;
      -run=*)
        tests_to_run+=("${arg}")
        ;;
      -save-logs=*)
        teardown_args+=("${arg}")
        ;;
      -teardown)
        teardown_args+=(-all)
        ;;
      -verbose)
        summary_format=standard-verbose
        ;;
      -help)
        usage
        return
        ;;
      --*)
        echo >&2 "cannot use flags with two leading dashes ('--...'), use single dashes instead ('-...')"
        return 1
        ;;
    esac
  done

  # By default use 30303 for fakepubsub.
  fakepubsub_node_port="30303"

  # If in CI (pull-test-infra-integration presubmit job), do some things slightly differently.
  if [[ -n "${ARTIFACTS:-}" ]]; then
    # Use the ARTIFACTS variable to save log output.
    teardown_args+=(-save-logs="${ARTIFACTS}/kind_logs")
    # Randomize the node port used for the fakepubsub service.
    fakepubsub_node_port="$(get_random_node_port)"
    log "Using randomized port ${fakepubsub_node_port} for fakepubsub"
  fi

  if [[ -n "${teardown_args[*]}" ]]; then
    # shellcheck disable=SC2064
    trap "${SCRIPT_ROOT}/teardown.sh ${teardown_args[*]}" EXIT
  fi

  for arg in "${setup_args[@]}"; do
    case "${arg}" in
      -setup-kind-cluster) setup_kind_cluster=1 ;;
      -setup-prow-components) setup_prow_components=1 ;;
      -build=*)
        build_images="${arg#-build=}"
        ;;
    esac
  done

  if ((setup_kind_cluster)); then
    "${SCRIPT_ROOT}"/setup-kind-cluster.sh \
      -fakepubsub-node-port="${fakepubsub_node_port}"
  fi

  if ((setup_prow_components)); then
    "${SCRIPT_ROOT}"/setup-prow-components.sh \
      ${build_images:+"-build=${build_images}"} \
      -fakepubsub-node-port="${fakepubsub_node_port}"
  fi

  build_gotestsum

  if [[ -n "${clear_args[*]}" ]]; then
    "${SCRIPT_ROOT}/clear.sh" "${clear_args[@]}"
  fi

  log "Finished preparing environment; running integration test"

  JUNIT_RESULT_DIR="${REPO_ROOT}/_output"
  # If we are in CI, copy to the artifact upload location.
  if [[ -n "${ARTIFACTS:-}" ]]; then
    JUNIT_RESULT_DIR="${ARTIFACTS}"
  fi

  # Run integration tests with junit output.
  mkdir -p "${JUNIT_RESULT_DIR}"
  "${REPO_ROOT}/_bin/gotestsum" \
    --format "${summary_format}" \
    --junitfile="${JUNIT_RESULT_DIR}/junit-integration.xml" \
    -- "${SCRIPT_ROOT}/test" \
    --run-integration-test ${tests_to_run[@]:+"${tests_to_run[@]}"} \
    --fakepubsub-node-port "${fakepubsub_node_port}"
}

function build_gotestsum() {
  log "Building gotestsum"
  set -x
  pushd "${REPO_ROOT}/hack/tools"
  go build -o "${REPO_ROOT}/_bin/gotestsum" gotest.tools/gotestsum
  popd
  set +x
}

main "$@"
