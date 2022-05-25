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
  fakegerritserver
  fakegitserver
  fakeghserver
  gerrit
  hook
  horologium
  prow-controller-manager
  sinker
  sub
  tide
)

# These are the images to build. The keys are the short (unique) image names,
# and the values are the paths from REPO_ROOT that define where the source code
# is located.
declare -rA PROW_IMAGES=(
  # Actual Prow components.
  [crier]=prow/cmd/crier
  [deck]=prow/cmd/deck
  [gerrit]=prow/cmd/gerrit
  [hook]=prow/cmd/hook
  [horologium]=prow/cmd/horologium
  [prow-controller-manager]=prow/cmd/prow-controller-manager
  [sinker]=prow/cmd/sinker
  [sub]=prow/cmd/sub
  [tide]=prow/cmd/tide
  # Fakes.
  [fakegerritserver]=prow/test/integration/cmd/fakegerritserver
  [fakegitserver]=prow/test/integration/cmd/fakegitserver
  [fakeghserver]=prow/test/integration/cmd/fakeghserver
  # Utility images. These images are not Prow components per se, and so do not
  # have corresponding Kubernetes configurations.
  [clonerefs]=prow/cmd/clonerefs
  [initupload]=prow/cmd/initupload
  [entrypoint]=prow/cmd/entrypoint
  [sidecar]=prow/cmd/sidecar
)

# Defines the one-to-many relationship between Prow images and components. This
# mapping tells us which Prow components need to be redeployed depending on what
# images are rebuilt.
declare -rA PROW_IMAGES_TO_COMPONENTS=(
  [crier]=crier
  [deck]="deck,deck-tenanted"
  [gerrit]=gerrit
  [hook]=hook
  [horologium]=horologium
  [prow-controller-manager]=prow-controller-manager
  [sinker]=sinker
  [sub]=sub
  [tide]=tide
  [fakegerritserver]=fakegerritserver
  [fakegitserver]=fakegitserver
  [fakeghserver]=fakeghserver
)

function do_kubectl() {
  kubectl --context="${_KIND_CONTEXT}" "$@"
}

function log() {
  >&2 cat <<EOF

==> $@

EOF
}
