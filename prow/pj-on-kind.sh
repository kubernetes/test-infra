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
  # Point kubectl at the mkpod cluster.
  export KUBECONFIG="${HOME}/.kube/kind-config-mkpod"
  parseArgs "$@"
  ensureInstall

  # Generate PJ and Pod.
  docker pull gcr.io/k8s-prow/mkpj:latest
  docker run -i --rm -v "${PWD}:${PWD}" -v "${config}:${config}" ${job_config_mnt} -w "${PWD}" gcr.io/k8s-prow/mkpj:latest "--config-path=${config}" "--job=${job}" ${job_config_flag} > "${PWD}/pj.yaml"
  docker pull gcr.io/k8s-prow/mkpod:latest
  docker run -i --rm -v "${PWD}:${PWD}" -w "${PWD}" gcr.io/k8s-prow/mkpod:latest --build-id=snowflake "--prow-job=${PWD}/pj.yaml" --local "--out-dir=${out_dir}/${job}" > "${PWD}/pod.yaml"

  # Add any k8s resources that the pod depends on to the kind cluster here. (secrets, configmaps, etc.)

  # Deploy pod and watch.
  echo "Applying pod to the mkpod cluster. Configure kubectl for the mkpod cluster with:"
  echo ">  export KUBECONFIG='${KUBECONFIG}'"
  pod=$(kubectl apply -f "${PWD}/pod.yaml" | cut -d ' ' -f 1)
  kubectl get "${pod}" -w
}

# Prep and check args.
function parseArgs() {
  # Use node mounts under /mnt/disks/ so pods behave well on COS nodes too. https://cloud.google.com/container-optimized-os/docs/concepts/disks-and-filesystem
  job="${1:-}"
  config="${CONFIG_PATH:-}"
  job_config_path="${JOB_CONFIG_PATH:-}"
  out_dir="${OUT_DIR:-/mnt/disks/prowjob-out}"
  kind_config="${KIND_CONFIG:-}"
  node_dir="${NODE_DIR:-/mnt/disks/kind-node}"  # Any pod hostPath mounts should be under this dir to reach the true host via the kind node.

  local new_only="  (Only used when creating a new kind cluster.)"
  echo "job=${job}"
  echo "CONFIG_PATH=${config}"
  echo "JOB_CONFIG_PATH=${job_config_path}"
  echo "OUT_DIR=${out_dir} ${new_only}"
  echo "KIND_CONFIG=${kind_config} ${new_only}"
  echo "NODE_DIR=${node_dir} ${new_only}"

  if [[ -z "${job}" ]]; then
    echo "Must specify a job name as the first argument."
    exit 2
  fi
  if [[ -z "${config}" ]]; then
    echo "Must specify config.yaml location via CONFIG_PATH env var."
    exit 2
  fi
  job_config_flag=""
  job_config_mnt=""
  if [[ -n "${job_config_path}" ]]; then
    job_config_flag="--job-config-path=${job_config_path}"
    job_config_mnt="-v ${job_config_path}:${job_config_path}"
  fi
}

# Ensures installation of prow tools, kind, and a kind cluster named "mkpod".
function ensureInstall() {
  # Install kind and set up cluster if not already done.
  if ! command -v kind >/dev/null 2>&1; then
    echo "Installing kind..."
    GO111MODULE="on" go get sigs.k8s.io/kind@v0.7.0
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
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - extraMounts:
      - containerPath: ${out_dir}
        hostPath: ${out_dir}
      # host <-> node mount for hostPath volumes in Pods. (All hostPaths should be under ${node_dir} to reach the host.)
      - containerPath: ${node_dir}
        hostPath: ${node_dir}
EOF
      kind create cluster --name=mkpod "--config=${temp_config}" --wait=5m
      rm "${temp_config}"
    fi
  fi
}

main "$@"
