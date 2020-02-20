#!/bin/bash
# Copyright 2016 The Kubernetes Authors.
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

# Download the latest version of kops, then run the e2e tests using e2e.sh.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

for i in {1..10}; do
  echo 'WARNING: kops-e2e-runner.sh is deprecated, migrate logic to kubetest'
done

if [[ -z "${KOPS_BASE_URL:-}" ]]; then
  readonly KOPS_LATEST=${KOPS_LATEST:-"latest-ci.txt"}
  readonly LATEST_URL="https://storage.googleapis.com/kops-ci/bin/${KOPS_LATEST}"
  export KOPS_BASE_URL=$(curl -fsS --retry 3 "${LATEST_URL}")
  if [[ -z "${KOPS_BASE_URL}" ]]; then
    echo "Can't fetch kops latest URL" >&2
    exit 1
  fi
fi

curl -fsS --retry 3 -o "/workspace/kops" "${KOPS_BASE_URL}/linux/amd64/kops"
chmod +x "/workspace/kops"

# Get kubectl on the path (works after e2e-runner.sh:unpack_binaries)
export PRIORITY_PATH="${WORKSPACE}/kubernetes/platforms/linux/amd64"

e2e_args=( \
  --deployment=kops \
  --kops=/workspace/kops \
)

# TODO(zmerlynn): This is duplicating some logic in e2e-runner.sh, but
# I'd rather keep it isolated for now.
if [[ "${KOPS_DEPLOY_LATEST_KUBE:-}" =~ ^[yY]$ ]]; then
  readonly KOPS_KUBE_LATEST_URL=${KOPS_DEPLOY_LATEST_URL:-"https://storage.googleapis.com/kubernetes-release-dev/ci/latest.txt"}
  readonly KOPS_KUBE_LATEST=$(curl -fsS --retry 3 "${KOPS_KUBE_LATEST_URL}")
  if [[ -z "${KOPS_KUBE_LATEST}" ]]; then
    echo "Can't fetch kube latest URL" >&2
    exit 1
  fi
  readonly KOPS_KUBE_RELEASE_URL=${KOPS_KUBE_RELEASE_URL:-"https://storage.googleapis.com/kubernetes-release-dev/ci"}

  e2e_args+=(--kops-kubernetes-version="${KOPS_KUBE_RELEASE_URL}/${KOPS_KUBE_LATEST}")
fi

# Define a custom instance lister for cluster/log-dump/log-dump.sh.
function log_dump_custom_get_instances() {
  local -r role=$1
  local kops_regions
  IFS=', ' read -r -a kops_regions <<< "${KOPS_REGIONS:-us-west-2}"
  for region in "${kops_regions[@]}"; do
    aws ec2 describe-instances \
      --region "${region}" \
      --filter \
        "Name=tag:KubernetesCluster,Values=$(kubectl config current-context)" \
        "Name=tag:k8s.io/role/${role},Values=1" \
        "Name=instance-state-name,Values=running" \
      --query "Reservations[].Instances[].PublicDnsName" \
      --output text
  done
}
pip install awscli # Only needed for log_dump_custom_get_instances
export -f log_dump_custom_get_instances # Export to cluster/log-dump/log-dump.sh

kubetest "${e2e_args[@]}" "${@}"

if [[ -n "${KOPS_PUBLISH_GREEN_PATH:-}" ]]; then

  if ! which gsutil; then
    export PATH=/google-cloud-sdk/bin:${PATH}
    if ! which gsutil; then
      echo "Can't find gsutil" >&2
      exit 1
    fi
  fi

  # TODO(krzyzacy) - debugging
  gcloud config list
  gcloud auth list

  echo "Publish version to ${KOPS_PUBLISH_GREEN_PATH}: ${KOPS_BASE_URL}"
  echo "${KOPS_BASE_URL}" | gsutil -h "Cache-Control:private, max-age=0, no-transform" cp - "${KOPS_PUBLISH_GREEN_PATH}"
fi
