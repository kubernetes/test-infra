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

if [[ -z "${KOPS_URL:-}" ]]; then
  readonly KOPS_LATEST=${KOPS_LATEST:-"latest-ci.txt"}
  readonly LATEST_URL="https://storage.googleapis.com/kops-ci/bin/${KOPS_LATEST}"
  readonly KOPS_URL=$(curl -fsS --retry 3 "${LATEST_URL}")
  if [[ -z "${KOPS_URL}" ]]; then
    echo "Can't fetch kops latest URL" >&2
    exit 1
  fi
fi

curl -fsS --retry 3 -o "${WORKSPACE}/kops" "${KOPS_URL}/linux/amd64/kops"
chmod +x "${WORKSPACE}/kops"
export NODEUP_URL="${KOPS_URL}/linux/amd64/nodeup"

# Get kubectl on the path (works after e2e-runner.sh:unpack_binaries)
export PATH="${PATH}:/workspace/kubernetes/platforms/linux/amd64"

export E2E_OPT="--deployment kops --kops /workspace/kops --kops-ssh-key /workspace/.ssh/kube_aws_rsa ${E2E_OPT}"

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

  export E2E_OPT="${E2E_OPT} --kops-kubernetes-version ${KOPS_KUBE_RELEASE_URL}/${KOPS_KUBE_LATEST}"
fi

$(dirname "${BASH_SOURCE}")/e2e-runner.sh

if [[ -n "${KOPS_PUBLISH_GREEN_PATH:-}" ]]; then
  export CLOUDSDK_CONFIG="/workspace/.config/gcloud"

  if ! which gsutil; then
    export PATH=/google-cloud-sdk/bin:${PATH}
    if ! which gsutil; then
      echo "Can't find gsutil" >&2
      exit 1
    fi
  fi
  echo "Publish version to ${KOPS_PUBLISH_GREEN_PATH}: ${KOPS_URL}"
  echo "${KOPS_URL}" | gsutil cp - "${KOPS_PUBLISH_GREEN_PATH}"
fi
