#!/bin/bash
# e2e-test.sh — orchestrates GPU e2e tests on a Lambda Cloud instance.
#
# Must be run from a kubernetes source checkout directory.
# Requires: LAMBDA_API_KEY_FILE, JOB_NAME, BUILD_ID, ARTIFACTS env vars.
# Optional: GPU_TYPE (default: gpu_1x_a100_sxm4)
set -o errexit
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR"
GPU_TYPE="${GPU_TYPE:-gpu_1x_a10}"
SSH_KEY_NAME="prow-${JOB_NAME}-${BUILD_ID}"

# --- Install lambdactl ---
GOPROXY=direct go install github.com/dims/lambdactl@latest

# --- Generate ephemeral SSH key ---
rm -f /tmp/lambda-ssh /tmp/lambda-ssh.pub
ssh-keygen -t ed25519 -f /tmp/lambda-ssh -N "" -q
SSH_KEY_ID=$(lambdactl --json ssh-keys add "${SSH_KEY_NAME}" /tmp/lambda-ssh.pub | jq -r '.id')

cleanup() {
  echo "Cleaning up..."
  [ -n "${INSTANCE_ID:-}" ] && lambdactl stop "${INSTANCE_ID}" --yes 2>/dev/null || true
  [ -n "${SSH_KEY_ID:-}" ] && lambdactl ssh-keys rm "${SSH_KEY_ID}" 2>/dev/null || true
}
trap cleanup EXIT

# --- Launch instance with retries ---
LAUNCH_OUTPUT=$(lambdactl --json start \
  --gpu "${GPU_TYPE}" \
  --ssh "${SSH_KEY_NAME}" \
  --name "prow-${BUILD_ID}" \
  --retries 4 \
  --retry-delay 60 \
  --wait-ssh)
INSTANCE_IP=$(echo "${LAUNCH_OUTPUT}" | jq -r '.ip')
INSTANCE_ID=$(echo "${LAUNCH_OUTPUT}" | jq -r '.id')

remote() { ssh ${SSH_OPTS} -i /tmp/lambda-ssh "ubuntu@${INSTANCE_IP}" "$@"; }
rsync_to() { rsync -e "ssh ${SSH_OPTS} -i /tmp/lambda-ssh" "$@"; }

# --- Build k8s binaries ---
git fetch --tags --depth 1 origin 2>/dev/null || true
KUBE_GIT_VERSION=$(git describe --tags --match='v*' 2>/dev/null || echo "v1.35.0")
make KUBE_GIT_VERSION="${KUBE_GIT_VERSION}" \
  WHAT="cmd/kubeadm cmd/kubelet cmd/kubectl test/e2e/e2e.test vendor/github.com/onsi/ginkgo/v2/ginkgo"

# --- Transfer binaries to Lambda instance ---
rsync_to _output/local/go/bin/{kubeadm,kubelet,kubectl,e2e.test,ginkgo} "ubuntu@${INSTANCE_IP}:/tmp/"

# --- Set up single-node k8s cluster with GPU support ---
remote bash -s < "${SCRIPT_DIR}/setup-cluster.sh"

# --- Run GPU e2e tests ---
remote bash -s <<TESTEOF
set -eux
export KUBECONFIG=\$HOME/.kube/config
mkdir -p /tmp/gpu-test-artifacts
/tmp/ginkgo \
  -timeout=60m \
  -focus="\[Feature:GPUDevicePlugin\]" \
  -skip="\[Flaky\]" \
  -v \
  /tmp/e2e.test \
  -- \
  --provider=aws \
  --kubeconfig=\$KUBECONFIG \
  --report-dir=/tmp/gpu-test-artifacts \
  --minStartupPods=8
TESTEOF

# --- Collect artifacts ---
mkdir -p "${ARTIFACTS}"
rsync_to "ubuntu@${INSTANCE_IP}:/tmp/gpu-test-artifacts/" "${ARTIFACTS}/" || true
