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

# Delete Kubernetes resources in the test cluster. This script guarantees that
# these operations are only executed against the test cluster, and so is safer
# than running kubectl directly (because your default context might be set to a
# production instance).

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_ROOT}"/lib.sh

function usage() {
  >&2 cat <<EOF
Delete Kubernetes resources in the test cluster.

Usage: $0 [options]

Examples:
  # Delete ProwJob CRs and test pods.
  $0 -all

  # Delete all ProwJob CRs.
  $0 -prowjobs

  # Delete all test pods.
  $0 -test-pods

Options:
    -all:
        Alias for -test-pods -prowjobs -components.

    -components:
        Delete all Prow components (deployments). This brings down the Prow
        components (pods) such that they are not restarted by Kubernetes (which
        would happen if you deleted just their pods).

    -test-data:
        Alias for -test-pods -prowjobs.

    -prowjobs:
        Delete all ProwJob CRs.

    -test-pods:
        Delete all test pods.

    -help:
        Display this help message.
EOF
}

function main() {
  declare -a delete_args

  if ! (($#)); then
    echo >&2 "teardown: missing flag: must provide one of -all, -prowjobs, or -test-pods"
    return 1
  fi

  for arg in "$@"; do
    case "${arg}" in
      -all)
        delete_args+=(-prowjobs)
        delete_args+=(-test-pods)
        delete_args+=(-components)
        ;;
      -prowjobs)
        delete_args+=("${arg}")
        ;;
      -test-pods)
        delete_args+=("${arg}")
        ;;
      -components)
        delete_args+=("${arg}")
        ;;
      -test-data)
        delete_args+=(-prowjobs)
        delete_args+=(-test-pods)
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

  if [[ -n "${delete_args[*]}" ]]; then
    if [[ " ${delete_args[*]} " =~ " -prowjobs " ]]; then
      delete_prowjobs
    fi
    if [[ " ${delete_args[*]} " =~ " -test-pods " ]]; then
      delete_test_pods
    fi
    if [[ " ${delete_args[*]} " =~ " -components " ]]; then
      delete_components
    fi
  fi
}

function delete_components() {
  log "KIND cluster: deleting all Prow components (deployments)"
  do_kubectl delete deployments.apps --all
}

function delete_test_pods() {
  log "KIND cluster: deleting all pods in the test-pods namespace"
  do_kubectl --namespace=test-pods delete pods --all
}

function delete_prowjobs() {
  log "KIND cluster: deleting all ProwJobs in the default namespace"
  do_kubectl delete prowjobs.prow.k8s.io --all
}

main "$@"
