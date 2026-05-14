#!/usr/bin/env bash
# Copyright The Kubernetes Authors.
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

# lambda-common.sh -- shared functions for Lambda Cloud GPU CI jobs.
# Source this file from project-specific e2e-test.sh scripts.
#
# Required env: JOB_NAME, BUILD_ID (set by Prow)
# Optional env: GPU_TYPE (default: gpu_1x_a10, set empty to accept any)

set -o errexit; set -o nounset; set -o pipefail

LAMBDA_SSH_OPTS=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR)
LAMBDA_GPU_TYPE="${GPU_TYPE-gpu_1x_a10}"
LAMBDA_SSH_DIR=""
LAMBDA_SSH_KEY=""
LAMBDA_SSH_KEY_ID=""
LAMBDA_INSTANCE_IP=""
LAMBDA_INSTANCE_ID=""

# Install lambdactl CLI
lambda_install_cli() {
  GOPROXY=direct go install github.com/dims/lambdactl@latest
}

# Create ephemeral SSH key and register with Lambda Cloud
lambda_create_ssh_key() {
  LAMBDA_SSH_DIR=$(mktemp -d /tmp/lambda-ssh.XXXXXX)
  LAMBDA_SSH_KEY="${LAMBDA_SSH_DIR}/key"
  local key_name
  key_name=$(echo -n "prow-${JOB_NAME}-${BUILD_ID}" | sha256sum | cut -c1-64)
  ssh-keygen -t ed25519 -f "${LAMBDA_SSH_KEY}" -N "" -q
  LAMBDA_SSH_KEY_ID=$(lambdactl --json ssh-keys add "${key_name}" "${LAMBDA_SSH_KEY}.pub" | jq -r '.id')
  export LAMBDA_SSH_KEY_NAME="${key_name}"
}

# Launch GPU instance and wait for SSH
lambda_launch() {
  local gpu_args=()
  if [ -n "${LAMBDA_GPU_TYPE}" ]; then
    gpu_args=(--gpu "${LAMBDA_GPU_TYPE}")
  fi
  local launch_output
  launch_output=$(lambdactl --json watch \
    "${gpu_args[@]}" \
    --ssh "${LAMBDA_SSH_KEY_NAME}" \
    --name "${LAMBDA_SSH_KEY_NAME}" \
    --interval 30 \
    --timeout 900 \
    --wait-ssh)
  LAMBDA_INSTANCE_IP=$(echo "${launch_output}" | jq -r '.ip')
  LAMBDA_INSTANCE_ID=$(echo "${launch_output}" | jq -r '.id')
  # Update LAMBDA_GPU_TYPE from the actual provisioned instance type.
  # When GPU_TYPE="" (accept any), the requested type is empty but the
  # provisioned type is concrete (e.g., gpu_1x_a10, gpu_8x_v100_n).
  # Callers use this to gate tests by GPU capability.
  LAMBDA_GPU_TYPE=$(echo "${launch_output}" | jq -r '.instance_type.name')
  echo "Launched ${LAMBDA_GPU_TYPE} instance ${LAMBDA_INSTANCE_ID} at ${LAMBDA_INSTANCE_IP}"
}

# Register cleanup trap (call after lambda_create_ssh_key)
lambda_register_cleanup() {
  trap '_lambda_cleanup' EXIT
}

_lambda_cleanup() {
  echo "Cleaning up Lambda resources..."
  [ -n "${LAMBDA_INSTANCE_ID:-}" ] && lambdactl stop "${LAMBDA_INSTANCE_ID}" --yes 2>/dev/null || true
  [ -n "${LAMBDA_SSH_KEY_ID:-}" ] && lambdactl ssh-keys rm "${LAMBDA_SSH_KEY_ID}" 2>/dev/null || true
  rm -rf "${LAMBDA_SSH_DIR}"
}

# SSH to the Lambda instance
lambda_remote() {
  ssh "${LAMBDA_SSH_OPTS[@]}" -i "${LAMBDA_SSH_KEY}" "ubuntu@${LAMBDA_INSTANCE_IP}" "$@"
}

# Rsync files to the Lambda instance (with retry for transient SSH failures)
lambda_rsync_to() {
  local attempt
  for attempt in 1 2 3; do
    if rsync -a -e "ssh ${LAMBDA_SSH_OPTS[*]} -i ${LAMBDA_SSH_KEY}" "$@"; then
      return 0
    fi
    echo "rsync attempt ${attempt} failed, retrying in 5s..." >&2
    sleep 5
  done
  echo "rsync failed after 3 attempts" >&2
  return 1
}

# Detect the Lambda instance's CPU architecture (call after lambda_launch).
# Sets LAMBDA_ARCH to "amd64" or "arm64".
lambda_detect_arch() {
  local uname_m
  uname_m=$(lambda_remote uname -m)
  case "${uname_m}" in
    x86_64)  LAMBDA_ARCH="amd64" ;;
    aarch64) LAMBDA_ARCH="arm64" ;;
    *)       LAMBDA_ARCH="amd64"; echo "WARNING: unknown arch '${uname_m}', defaulting to amd64" ;;
  esac
  echo "Detected Lambda instance architecture: ${LAMBDA_ARCH} (uname -m: ${uname_m})"
}

# Download k8s release binaries to a local directory
# Usage: lambda_download_k8s /tmp/k8s-bins [version]
# Requires: LAMBDA_ARCH set (call lambda_detect_arch first)
lambda_download_k8s() {
  local dest="$1"
  local version="${2:-$(curl -sSfL https://dl.k8s.io/release/stable.txt)}"
  local arch="${LAMBDA_ARCH:-amd64}"
  mkdir -p "${dest}"
  for bin in kubeadm kubelet kubectl; do
    curl -sSfL "https://dl.k8s.io/release/${version}/bin/linux/${arch}/${bin}" \
      -o "${dest}/${bin}"
    chmod +x "${dest}/${bin}"
  done
  echo "Downloaded k8s ${version} binaries (${arch}) to ${dest}"
}

# Collect artifacts from the Lambda instance
# Usage: lambda_collect_artifacts /remote/path [/local/artifacts]
lambda_collect_artifacts() {
  local remote_path="$1"
  local local_path="${2:-${ARTIFACTS}}"
  mkdir -p "${local_path}"
  lambda_rsync_to "ubuntu@${LAMBDA_INSTANCE_IP}:${remote_path}" "${local_path}/" || true
}

# Convenience: init everything (install CLI, create key, register cleanup, launch, detect arch)
lambda_init_and_launch() {
  lambda_install_cli
  lambda_create_ssh_key
  lambda_register_cleanup
  lambda_launch
  lambda_detect_arch
}
