#!/bin/bash
# Copyright 2018 The Kubernetes Authors.
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

# bump.sh will
# * ensure there are no pending changes
# * optionally activate GOOGLE_APPLICATION_CREDENTIALS and configure-docker if set
# * run //prow:release-push to build and push prow images
# * update all the cluster/*.yaml files to use the new image tags

set -o errexit
set -o nounset
set -o pipefail

# TODO(fejta): rewrite this in a better language REAL SOON
# TODO(fejta): update this and hack/print-workspace-status to customize STABLE_*_CLUSTER

if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
  echo "Detected GOOGLE_APPLICATION_CREDENTIALS, activating..." >&2
  gcloud auth activate-service-account --key-file="$GOOGLE_APPLICATION_CREDENTIALS"
fi

case "${1:-}" in
"--confirm")
  shift
  ;;
*)
  read -p "Deploy prow to prod [no]: " confirm
  if [[ "${confirm}" != y* ]]; then
    exit 1
  fi
  ;;
esac

# See https://misc.flogisoft.com/bash/tip_colors_and_formatting

color-green() { # Green
  echo -e "\x1B[1;32m${@}\x1B[0m"
}

color-context() { # Bold blue
  echo -e "\x1B[1;34m${@}\x1B[0m"
}

color-missing() { # Yellow
  echo -e "\x1B[1;33m${@}\x1B[0m"
}

ensure-context() {
  local proj=$1
  local zone=$2
  local cluster=$3
  local context="gke_${proj}_${zone}_${cluster}"
  echo -n " $(color-context "$context")"
  kubectl config get-contexts "$context" &> /dev/null && return 0
  echo ": $(color-missing MISSING), getting credentials..."
  gcloud container clusters get-credentials --project="$proj" --zone="$zone" "$cluster"
  kubectl config get-contexts "$context" > /dev/null
  echo -n "Ensuring contexts exist:"
}

echo -n "Ensuring contexts exist:"
current_context=$(kubectl config current-context 2>/dev/null || true)
restore-context() {
  if [[ -n "$current_context" ]]; then
    kubectl config set-context "$current_context"
  fi
}
trap restore-context EXIT
ensure-context k8s-prow-builds us-central1-f prow
ensure-context k8s-prow us-central1-f prow
echo " $(color-green done), Deploying prow..."
for s in {5..1}; do
    echo -n $'\r'"in $s..."
    sleep 1s
done
bazel run //config/prow/cluster:production.apply --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64 "$@"
bazel run //config/prow/cluster/monitoring:prow_monitoring_objects.apply --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64 "$@"
bazel run //config/prow/cluster/monitoring:production.apply --platforms=@io_bazel_rules_go//go/toolchain:linux_amd64 "$@"
echo "$(color-green SUCCESS)"
