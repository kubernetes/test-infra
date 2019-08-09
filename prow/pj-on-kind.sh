#!/usr/bin/env bash
# Copyright 2019 The Kubernetes Authors.
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

# Requires go, docker, and kubectl.

set -o errexit
set -o nounset
set -o pipefail

function main() {
  parseArgs "$@"
  ensureInstall

  # Generate PJ and Pod.
  mkpj "--config-path=${config}" "${job_config}" "--job=${job}" > "${PWD}/pj.yaml"
  mkpod --build-id=snowflake "--prow-job=${PWD}/pj.yaml" --local "--out-dir=/output/${job}" > "${PWD}/pod.yaml"

  # Deploy pod and watch.
  echo "Applying pod to the mkpod cluster. Configure kubectl for the mkpod cluster with:"
  echo '>  export KUBECONFIG="$(kind get kubeconfig-path --name="mkpod")"'
  pod=$(kubectl apply -f "${PWD}/pod.yaml" | cut -d ' ' -f 1)
  kubectl get "${pod}" -w
}

# Prep and check args.
function parseArgs() {
  job="${1:-""}"
  config="${CONFIG_PATH:-""}"
  job_config="${JOB_CONFIG_PATH:-""}"
  out_dir="${OUT_DIR:-"/tmp/prowjob-out"}"
  kind_config="${KIND_CONFIG:-""}"

  local new_only="  (Only used when creating a new kind cluster.)"
  echo "job=${job}"
  echo "CONFIG_PATH=${config}"
  echo "JOB_CONFIG_PATH=${job_config}"
  echo "OUT_DIR=${out_dir} ${new_only}"
  echo "KIND_CONFIG=${kind_config} ${new_only}"

  if [[ -z "${job}" ]]; then
    echo "Must specify a job name as the first argument."
    exit 2
  fi
  if [[ -z "${config}" ]]; then
    echo "Must specify config.yaml location via CONFIG_PATH env var."
    exit 2
  fi
  if [[ -n "${job_config}" ]]; then
    job_config="--job-config-path=${job_config}"
  fi
}

# Ensures installation of prow tools, kind, and a kind cluster named "mkpod".
function ensureInstall() {
  # Install mkpj and mkpod if not already done.
  if ! command -v mkpj >/dev/null 2>&1; then
    echo "Installing mkpj..."
    go get k8s.io/test-infra/prow/cmd/mkpj
  fi
  if ! command -v mkpod >/dev/null 2>&1; then
    echo "Installing mkpod..."
    go get k8s.io/test-infra/prow/cmd/mkpod
  fi

  # Install kind and set up cluster if not already done.
  if ! command -v kind >/dev/null 2>&1; then
    echo "Installing kind..."
    GO111MODULE="on" go get sigs.k8s.io/kind@v0.4.0
  fi
  local found="false"
  for clust in $(kind get clusters); do
    if [[ "${clust}" == "mkpod" ]]; then
      found="true"
      break
    fi
  done
  if [[ "${found}" == "false" ]]; then
    # Need to create the "mkpod" kind cluster.
    if [[ -n "${kind_config}" ]]; then
      kind create cluster --name=mkpod "--config=${kind_config}" --wait=5m
    else
      # Create a temporary kind config file.
      local temp_config="${PWD}/temp-mkpod-kind-config.yaml"
      cat <<EOF > "${temp_config}"
kind: Cluster
apiVersion: kind.sigs.k8s.io/v1alpha3
nodes:
  - extraMounts:
      - containerPath: /output
        hostPath: ${out_dir}
EOF
      kind create cluster --name=mkpod "--config=${temp_config}" --wait=5m
      rm "${temp_config}"
    fi
  fi
  # Point kubectl at the mkpod cluster.
  export KUBECONFIG="$(kind get kubeconfig-path --name="mkpod")"
}

main "$@"