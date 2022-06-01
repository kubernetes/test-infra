#!/usr/bin/env bash
# Copyright 2022 The Kubernetes Authors.
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

# Set up the KIND cluster.

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT="$(cd "$(dirname "$0")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_ROOT}"/lib.sh

function usage() {
  >&2 cat <<EOF
Build Prow components and deploy them into the KIND test cluster.

Usage: $0 [options]

Examples:
  # Recompile all Prow components and deploy them.
  $0

  # Recompile fakegitserver and deploy it to Prow.
  $0 -build=fakegitserver

Options:
    -build='':
        Build only the comma-separated list of Prow components with
        "${REPO_ROOT}"/hack/prowimagebuilder. Useful when developing a fake
        service that needs frequent recompilation. The images are a
        comma-separated string. Also results in only redeploying certain entries
        in PROW_COMPONENTS, by way of PROW_IMAGES_TO_COMPONENTS in lib.sh.

    -help:
        Display this help message.
EOF
}

function main() {
  declare -a build_images
  local build_images_val

  for arg in "$@"; do
    case "${arg}" in
      -build=*)
        build_images_val="${arg#-build=}"
        for image in ${build_images_val//,/ }; do
          build_images+=("${image}")
        done
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
  build_prow_images "${build_images[@]}"
  remove_old_components "${build_images[@]}"
  deploy_prow
}

function build_prow_images() {
  declare -a build_images
  local prowimagebuilder_yaml

  if (($#)); then
    log "Building select Prow images"
    for image in "${@}"; do
      build_images+=("${image}")
    done
  else
    log "Building *all* Prow images"
    for image in "${!PROW_IMAGES[@]}"; do
      build_images+=("${image}")
    done
  fi

  prowimagebuilder_yaml="$(create_prowimagebuilder_yaml "${build_images[@]}")"
  # shellcheck disable=SC2064
  trap "rm -f ${prowimagebuilder_yaml}" EXIT SIGINT SIGTERM

  >&2 cat <<EOF
==> ${prowimagebuilder_yaml} contents:

\`\`\`
$(cat "${prowimagebuilder_yaml}")
\`\`\`

EOF

  set -x
  go run \
    "${REPO_ROOT}"/hack/prowimagebuilder \
    --ko-docker-repo="localhost:${LOCAL_DOCKER_REGISTRY_PORT}" \
    --prow-images-file="${prowimagebuilder_yaml}" \
    --push
  set +x
  log "Finished building images"
}

function create_prowimagebuilder_yaml() {
  # Create a definitive reference of valid prow components (images) that can be
  # built by prowimagebuilder.
  local tmpfile
  tmpfile=$(mktemp /tmp/prowimagebuilder.XXXXXX.yaml)

  echo "images:" >> "${tmpfile}"

  for arg in "$@"; do
    if [[ -v "PROW_IMAGES[${arg}]" ]]; then
      echo "  - dir: ${PROW_IMAGES[${arg}]}" >> "${tmpfile}"
    else
      echo >&2 "Unrecognized prow component \"${arg}\""
      return 1
    fi
  done
  echo "${tmpfile}"
}

function remove_old_components() {
  declare -a prow_components
  local prow_component
  if (($#)); then
    log "Removing select Prow components"
    for image in "$@"; do
      if [[ -v "PROW_IMAGES_TO_COMPONENTS[${image}]" ]]; then
        for prow_component in ${PROW_IMAGES_TO_COMPONENTS[${image}]//,/ }; do
          prow_components+=("${prow_component}")
        done
      fi
    done

  else
    log "Removing *all* Prow components (if any)"
    for prow_component in "${PROW_COMPONENTS[@]}"; do
      prow_components+=("${prow_component}")
    done
  fi

  for prow_component in "${prow_components[@]}"; do
    do_kubectl delete deployment -l app="${prow_component}"
    do_kubectl delete pods -l app="${prow_component}"
  done
}

function deploy_prow() {
  local prow_component
  local component_ready
  log "Deploying prow component(s)"

  # Even though we apply the entire Prow configuration, Kubernetes is smart
  # enough to only redeploy those components who configurations have changed as
  # a result of newly built images (from build_prow_images()).
  pushd "${SCRIPT_ROOT}/config/prow"
  do_kubectl create configmap config --from-file=./config.yaml --dry-run=client -oyaml | do_kubectl apply -f -
  do_kubectl create configmap plugins --from-file=./plugins.yaml --dry-run=client -oyaml | do_kubectl apply -f -
  do_kubectl create configmap job-config --from-file=./jobs --dry-run=client -oyaml | do_kubectl apply -f -
  do_kubectl apply --server-side=true -f ./cluster
  popd

  log "Waiting for Prow components"
  for prow_component in "${PROW_COMPONENTS[@]}"; do
    component_ready=0
    >&2 echo "Waiting for ${prow_component}"
    for _ in $(seq 1 180); do
      if do_kubectl wait pod \
        --for=condition=ready \
        --selector=app="${prow_component}" \
        --timeout=180s; then
        component_ready=1
        break
      else
        sleep 1
      fi
    done

    # If a component fails to start up and we're in CI, record logs.
    if ! ((component_ready)) && [[ -n "${ARTIFACTS:-}" ]]; then
      >&2 do_kubectl get pods
      "${SCRIPT_ROOT}/teardown.sh" "-save-logs=${ARTIFACTS}/kind_logs"
      return 1
    fi
  done

  log "Prow components are ready"
}

main "$@"
