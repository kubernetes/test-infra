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
  # Deploy all Prow components without building anything. This fails if the
  # images the components rely on have not yet been built by ko.
  $0

  # Build all images required by Prow components, and deploy them.
  $0 -build=ALL

  # Build only the fakegitserver image and deploy it to Prow.
  $0 -build=fakegitserver

  # Build only the fakegitserver and fakegerritserver images and deploy them to
  # Prow.
  $0 -build=fakegitserver,fakegerritserver

  # Redeploy fakegitserver to Prow, without building it, by deleting the current
  # pods associated with it. This fails if this component has not been built
  # yet.
  $0 -delete=fakegitserver

  # Delete all Prow components from the cluster, then deploy them all back
  # again. This is useful if you want to force pods to restart from a blank
  # slate.
  $0 -delete=ALL

  # Delete *ALL* components, recompile them all, and finally deploy everything
  # again.

  $0 -delete=ALL -build=ALL

Options:
    -build='':
        Build only the comma-separated list of Prow components with
        "${REPO_ROOT}"/hack/prowimagebuilder. Useful when developing a fake
        service that needs frequent recompilation. The images are a
        comma-separated string.

        The value "ALL" for this falg is an alias for all images (PROW_IMAGES in
        lib.sh).

    -delete='':
        Force the deletion of the given (currently deployed) Prow components by
        deleting their associated pods. The value "ALL" for this flag is an
        alias for all components (PROW_COMPONENTS in lib.sh).

        You only need to use this flag if you want to force the given components
        to start from a blank state (e.g., you want to clear its memory for
        whatever reason). Technically, you can delete pods manually with kubectl
        to achieve the same effect; this flag is given here as a convenience.

    -help:
        Display this help message.
EOF
}

function main() {
  declare -a images
  declare -a components
  local images_val
  local components_val

  for arg in "$@"; do
    case "${arg}" in
      -build=*)
        images_val="${arg#-build=}"
        for image in ${images_val//,/ }; do
          if [[ "${image}" == ALL ]]; then
            images=("${!PROW_IMAGES[@]}")
            break
          else
            images+=("${image}")
          fi
        done
        ;;
      -delete=*)
        components_val="${arg#-delete=}"
        for component in ${components_val//,/ }; do
          if [[ "${component}" == ALL ]]; then
            components=("${PROW_COMPONENTS[@]}")
            break
          else
            components+=("${component}")
          fi
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

  if [[ -n "${images[*]}" ]]; then
    build_prow_images "${images[@]}"
  fi

  if [[ -n "${components[*]}" ]]; then
    delete_components "${components[@]}"
  fi

  deploy_prow
}

function build_prow_images() {
  declare -a images
  local prowimagebuilder_yaml

  if (($#)); then
    log "Building select Prow images"
    for image in "${@}"; do
      images+=("${image}")
    done
  else
    log "Building *all* Prow images"
    for image in "${!PROW_IMAGES[@]}"; do
      images+=("${image}")
    done
  fi

  prowimagebuilder_yaml="$(create_prowimagebuilder_yaml "${images[@]}")"
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

function delete_components() {
  local component
  if ! (($#)); then
    log "(Prow components) nothing to delete"
    return
  fi

  log "Deleting Prow components: $*"
  for component in "$@"; do
    do_kubectl delete deployment -l app="${component}"
    do_kubectl delete pods -l app="${component}"
  done
}

# deploy_prow applies the full Kubernetes configuration for all components. If
# any component's images have changed (recompiled and republished to the logal
# registry by ko), then they will be picked up and Kubernetes will restart those
# affected pods.
function deploy_prow() {
  local component
  local component_ready
  log "Deploying Prow components"

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
  for component in "${PROW_COMPONENTS[@]}"; do
    component_ready=0
    >&2 echo "Waiting for ${component}"
    for _ in $(seq 1 180); do
      if do_kubectl wait pod \
        --for=condition=ready \
        --selector=app="${component}" \
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
