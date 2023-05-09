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

# shellcheck disable=SC2034

SCRIPT_ROOT="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_ROOT}/../../.." && pwd -P)"

# Default variables. Note that these variables are not environment variables and
# are local to this script and other scripts that source this script (that is,
# even if you change them outside of this script, they are ignored as they are
# redeclared here).
#
readonly _KIND_CLUSTER_NAME="kind-prow-integration"
readonly _KIND_CONTEXT="kind-${_KIND_CLUSTER_NAME}"
readonly LOCAL_DOCKER_REGISTRY_NAME="${_KIND_CLUSTER_NAME}-registry"
readonly LOCAL_DOCKER_REGISTRY_PORT="5001"

# These are the components to test (by default). These are the services that
# must be deployed into the test cluster in order to test all integration tests.
#
# Note that some of these components use the same image. For example, deck and
# deck-tenanted both use the "deck" image in PROW_IMAGES.
declare -ra PROW_COMPONENTS=(
  crier
  deck
  deck-tenanted
  fakegcsserver
  fakegerritserver
  fakegitserver
  fakeghserver
  fakepubsub
  gerrit
  hook
  horologium
  prow-controller-manager
  sinker
  sub
  tide
  webhook-server
)

# These are the images to build. The keys are the short (unique) image names,
# and the values are the paths from REPO_ROOT that define where the source code
# is located.
declare -rA PROW_IMAGES=(
  # Actual Prow components.
  [crier]=prow/cmd/crier
  [deck]=prow/cmd/deck
  [gangway]=prow/cmd/gangway
  [gerrit]=prow/cmd/gerrit
  [hook]=prow/cmd/hook
  [horologium]=prow/cmd/horologium
  [prow-controller-manager]=prow/cmd/prow-controller-manager
  [sinker]=prow/cmd/sinker
  [sub]=prow/cmd/sub
  [tide]=prow/cmd/tide
  # Fakes.
  [fakegcsserver]=prow/test/integration/cmd/fakegcsserver
  [fakegerritserver]=prow/test/integration/cmd/fakegerritserver
  [fakegitserver]=prow/test/integration/cmd/fakegitserver
  [fakeghserver]=prow/test/integration/cmd/fakeghserver
  [fakepubsub]=prow/test/integration/cmd/fakepubsub
  # Utility images. These images are not Prow components per se, and so do not
  # have corresponding Kubernetes configurations.
  [clonerefs]=prow/cmd/clonerefs
  [initupload]=prow/cmd/initupload
  [entrypoint]=prow/cmd/entrypoint
  [sidecar]=prow/cmd/sidecar
  [webhook-server]=prow/cmd/webhook-server
)

# Defines the one-to-many relationship between Prow images and components. This
# mapping tells us which Prow components need to be redeployed depending on what
# images are rebuilt.
declare -rA PROW_IMAGES_TO_COMPONENTS=(
  [crier]=crier
  [deck]="deck,deck-tenanted"
  [gangway]=gangway
  [gerrit]=gerrit
  [hook]=hook
  [horologium]=horologium
  [prow-controller-manager]=prow-controller-manager
  [sinker]=sinker
  [sub]=sub
  [tide]=tide
  [fakegcsserver]=fakegcsserver
  [fakegerritserver]=fakegerritserver
  [fakegitserver]=fakegitserver
  [fakeghserver]=fakeghserver
  [fakepubsub]=fakepubsub
  [webhook-server]=webhook-server
)

# Defines the order in which we'll start and wait for components to be ready.
# Each element is deployed in order. If we encounter a WAIT value, we wait until
# the component is ready before proceeding with further deployments.
declare -ra PROW_DEPLOYMENT_ORDER=(
  # Start up basic, dependency-free components (and non-components like secrets,
  # ingress, etc) first.
  50_crd.yaml
  WAIT_FOR_CRD_prowjobs.prow.k8s.io,default

  100_starter.yaml
  WAIT_FOR_RESOURCE_namespaces,test-pods,default
  WAIT_FOR_RESOURCE_secrets,oauth-token,default
  WAIT_FOR_RESOURCE_secrets,kubeconfig,default

  101_secrets.yaml
  WAIT_FOR_RESOURCE_secrets,hmac-token,default
  WAIT_FOR_RESOURCE_secrets,http-cookiefile,default
  WAIT_FOR_RESOURCE_secrets,cookie,default
  WAIT_FOR_RESOURCE_secrets,github-oauth-config,default

  200_ingress.yaml
  WAIT_FOR_RESOURCE_ingresses,strip-path-prefix,default
  WAIT_FOR_RESOURCE_ingresses,no-strip-path-prefix,default

  # Create ghserver early, because other things depend on it. Otherwise we end
  # up logging a lot of errors about failing to connect to a fake service (e.g.,
  # fakeghserver) because it is not running yet. Connection failures slow down
  # the startup time a bit because they can lead to exponential backoffs until
  # the connections succeed.
  fakeghserver.yaml
  WAIT_fakeghserver

  # Start fakepubsub early, but don't wait for it just yet. This is because this
  # is a big image and if the local registry is empty (we're running integraion
  # tests on a cold machine), it takes a long time for the deployment to pull it
  # from the local registry.
  fakepubsub.yaml
  # Sub can't properly start its PullServer unless the subscriptions have
  # already been created. So wait for fakepubsub to be initialized with those
  # subscriptions first.
  WAIT_fakepubsub

  fakegcsserver.yaml
  WAIT_fakegcsserver

  fakegerritserver.yaml
  WAIT_fakegerritserver

  fakegitserver.yaml
  WAIT_fakegitserver

  gerrit.yaml
  WAIT_FOR_RESOURCE_roles,gerrit,default
  WAIT_FOR_RESOURCE_rolebindings,gerrit,default
  WAIT_FOR_RESOURCE_serviceaccounts,gerrit,default
  WAIT_gerrit

  horologium_rbac.yaml
  horologium_service.yaml
  horologium_deployment.yaml
  WAIT_FOR_RESOURCE_roles,horologium,default
  WAIT_FOR_RESOURCE_rolebindings,horologium,default
  WAIT_FOR_RESOURCE_serviceaccounts,horologium,default
  WAIT_horologium

  prow_controller_manager_rbac.yaml
  prow_controller_manager_service.yaml
  prow_controller_manager_deployment.yaml
  WAIT_FOR_RESOURCE_roles,prow-controller-manager,default
  WAIT_FOR_RESOURCE_roles,prow-controller-manager,test-pods
  WAIT_FOR_RESOURCE_rolebindings,prow-controller-manager,default
  WAIT_FOR_RESOURCE_rolebindings,prow-controller-manager,test-pods
  WAIT_FOR_RESOURCE_serviceaccounts,prow-controller-manager,default
  WAIT_prow-controller-manager

  sinker_rbac.yaml
  sinker_service.yaml
  sinker.yaml
  WAIT_FOR_RESOURCE_roles,sinker,default
  WAIT_FOR_RESOURCE_roles,sinker,test-pods
  WAIT_FOR_RESOURCE_rolebindings,sinker,default
  WAIT_FOR_RESOURCE_rolebindings,sinker,test-pods
  WAIT_FOR_RESOURCE_serviceaccounts,sinker,default
  WAIT_sinker

  # Deploy hook and tide early because crier, deck, etc. depend on them.
  hook_rbac.yaml
  hook_service.yaml
  hook_deployment.yaml
  WAIT_FOR_RESOURCE_roles,hook,default
  WAIT_FOR_RESOURCE_rolebindings,hook,default
  WAIT_FOR_RESOURCE_serviceaccounts,hook,default
  WAIT_hook

  tide_rbac.yaml
  tide_service.yaml
  tide_deployment.yaml
  WAIT_FOR_RESOURCE_roles,tide,default
  WAIT_FOR_RESOURCE_rolebindings,tide,default
  WAIT_FOR_RESOURCE_serviceaccounts,tide,default
  WAIT_tide

  crier_rbac.yaml
  crier_service.yaml
  crier_deployment.yaml
  WAIT_FOR_RESOURCE_roles,crier,default
  WAIT_FOR_RESOURCE_roles,crier,test-pods
  WAIT_FOR_RESOURCE_rolebindings,crier-namespaced,default
  WAIT_FOR_RESOURCE_rolebindings,crier-namespaced,test-pods
  WAIT_FOR_RESOURCE_serviceaccounts,crier,default
  WAIT_crier

  deck_rbac.yaml
  deck_service.yaml
  deck_deployment.yaml
  deck_tenant_deployment.yaml
  WAIT_FOR_RESOURCE_roles,deck,default
  WAIT_FOR_RESOURCE_roles,deck,test-pods
  WAIT_FOR_RESOURCE_rolebindings,deck,default
  WAIT_FOR_RESOURCE_rolebindings,deck,test-pods
  WAIT_FOR_RESOURCE_serviceaccounts,deck,default
  WAIT_deck
  WAIT_deck-tenanted

  webhook_server_rbac.yaml
  webhook_server_service.yaml
  webhook_server_deployment.yaml
  WAIT_FOR_RESOURCE_clusterroles,webhook-server,default
  WAIT_FOR_RESOURCE_clusterrolebindings,webhook-server,default
  WAIT_FOR_RESOURCE_serviceaccounts,webhook-server,default
  WAIT_webhook-server

  gangway_rbac.yaml
  gangway_service.yaml
  gangway_deployment.yaml
  WAIT_FOR_RESOURCE_roles,gangway,default
  WAIT_FOR_RESOURCE_rolebindings,gangway,default
  WAIT_FOR_RESOURCE_serviceaccounts,gangway,default
  WAIT_gangway

  sub.yaml
  WAIT_FOR_RESOURCE_roles,sub,default
  WAIT_FOR_RESOURCE_rolebindings,sub,default
  WAIT_FOR_RESOURCE_serviceaccounts,sub,default
  WAIT_sub
)

function do_kubectl() {
  kubectl --context="${_KIND_CONTEXT}" "$@"
}

function log() {
  >&2 cat <<EOF

==> $@

EOF
}

function wait_for_readiness() {
  local component

  component="${1}"

  echo >&2 "Waiting for ${component}"
  for _ in $(seq 1 180); do
    if  >/dev/null 2>&1 do_kubectl wait pod \
      --for=condition=ready \
      --selector=app="${component}" \
      --namespace=default \
      --timeout=5s; then
      return
    else
      echo >&2 "waiting..."
      sleep 1
    fi
  done

  echo >&2 "${component} failed to get ready"
  return 1
}

function wait_for_resource() {
  local arg
  local resource
  local name
  local namespace

  arg="${1:-}"

  declare -a args
  # shellcheck disable=SC2206
  args=(${arg//,/ })
  if ((${#args[@]} != 3)); then
    echo >&2 "wanted a CSV of exactly 3 values, with the syntax '<RESOURCE>,<NAME>,<NAMESPACE>'; got '${arg}'"
  fi

  resource="${args[0]}"
  name="${args[1]}"
  namespace="${args[2]}"

  echo >&2 "waiting for ${resource}/${name} in namespace '${namespace}'..."

  # Check to see that the named resource exists in the given namespace. Time out
  # after ~10 seconds.
  for _ in $(seq 1 10); do
    if >/dev/null do_kubectl get -n "${namespace}" "${resource}" "${name}"; then
      return 0
    fi
    sleep 1
    echo >&2 "waiting for ${resource}/${name} in namespace '${namespace}'..."
  done

  return 1
}

function wait_for_crd() {
  local arg
  local name
  local namespace

  arg="${1:-}"

  declare -a args
  # shellcheck disable=SC2206
  args=(${arg//,/ })
  if ((${#args[@]} != 2)); then
    echo >&2 "wanted a CSV of exactly 2 values, with the syntax '<NAME>,<NAMESPACE>'; got '${arg}'"
  fi

  name="${args[0]}"
  namespace="${args[1]}"

  echo >&2 "waiting for CRD ${name} in namespace '${namespace}'..."

  for _ in $(seq 1 10); do
    if >/dev/null do_kubectl wait -n "${namespace}" --for condition=established --timeout=120s crd "${name}"; then
      return 0
    fi
    sleep 1
    echo >&2 "waiting for CRD ${name} in namespace '${namespace}'..."
  done

  return 1
}

function get_random_node_port() {
  # 30000-32767 is the default NodePort range. If "shuf" isn't available, use
  # 30303 as a default.
  shuf -i 30000-32767 -n 1 || echo 30303
}
