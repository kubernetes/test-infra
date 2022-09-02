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

# Delete the KIND cluster used for integration tests.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_ROOT}"/lib.sh

function usage() {
  >&2 cat <<EOF
Tear down resources created by the integration tests.

Usage: $0 [options]

Examples:
  # Tear down all resources (KIND cluster and local Docker registry).
  $0 -all

  # Tear down only the KIND cluster.
  $0 -kind-cluster

  # Tear down only the local Docker registry.
  $0 -local-registry

  # Save KIND logs (these can be verbose) to directory "./logs".
  $0 -save-logs=logs

Options:
    -all:
        Delete both the KIND cluster and local Docker registry. These two
        resources are ultimately just Docker containers. You can check if these
        have been deleted by watching "docker ps -a" output.

    -kind-cluster:
        Only delete the KIND cluster (container named "${_KIND_CLUSTER_NAME}-control-plane").

    -local-registry:
        Only delete the local Docker registry (container named
        "${LOCAL_DOCKER_REGISTRY_NAME}").

    -save-logs='':
        Save KIND cluster logs to the given directory before tearing anything
        down. This saves all log output of all containers in all namespaces.

        This flag gets implied if the ARTIFACTS environment variable is set.

    -help:
        Display this help message.
EOF
}

function main() {
  local artifacts_dir
  declare -a teardown_args

  if ! (($#)); then
    echo >&2 "teardown: missing flag: must provide one of -all, -kind-cluster, or -local-registry"
    return 1
  fi

  for arg in "$@"; do
    case "${arg}" in
      -all)
        teardown_args+=(-kind-cluster)
        teardown_args+=(-local-registry)
        ;;
      -kind-cluster)
        teardown_args+=("${arg}")
        ;;
      -local-registry)
        teardown_args+=("${arg}")
        ;;
      -save-logs=*)
        artifacts_dir="${arg#-save-logs=}"
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

  if [[ -n "${artifacts_dir:-}" ]]; then
    save_logs "${artifacts_dir}"
  fi

  if [[ -n "${teardown_args[*]}" ]]; then
    if [[ " ${teardown_args[*]} " =~ " -kind-cluster " ]]; then
      teardown_kind_cluster
    fi
    if [[ " ${teardown_args[*]} " =~ " -local-registry " ]]; then
      teardown_local_registry
    fi
  fi
}

function save_logs() {
  local log_dir
  log_dir="${1:-}"

  # Grab KIND logs. KIND does a thorough job of exporting all logs from all pods
  # across all namespaces, including previously deleted (failed) containers, so
  # this is much better than what we can manually collect with kubectl.
  #
  # TODO(listx): Make horologium_test.go delete the 1-minute periodic job as
  # part of cleanup.
  # TODO(listx): Make horologium_test.go name its jobs after its test name.
  log "Saving logs to ${log_dir}"
  kind export logs --name "${_KIND_CLUSTER_NAME}" "${log_dir}" || true
}

function teardown_kind_cluster() {
  log "Tearing down the KIND cluster"
  kind delete cluster --name "${_KIND_CLUSTER_NAME}" || true
}
function teardown_local_registry() {
  log "Deleting local registry (if any)"
  docker stop "${LOCAL_DOCKER_REGISTRY_NAME}" 2>/dev/null || true
  docker rm -f "${LOCAL_DOCKER_REGISTRY_NAME}" 2>/dev/null || true
}

main "$@"
