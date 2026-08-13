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

# e2e-test.sh -- orchestrates GPU e2e tests on a Lambda Cloud instance.
#
# Must be run from a kubernetes source checkout directory.
# Requires: LAMBDA_API_KEY_FILE, JOB_NAME, BUILD_ID, ARTIFACTS env vars.
# Optional: GPU_TYPE (default: gpu_1x_a10, set empty to accept any available)
set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/lib/lambda-common.sh"

# --- Launch Lambda instance ---
lambda_init_and_launch

# --- Build k8s binaries ---
# Install the arm64 cross-compile toolchain when targeting arm64 from an
# amd64 build host. kubelet's cgo code paths cannot be compiled with
# CGO_ENABLED=0, so we need a real cross-gcc.
if [ "${LAMBDA_ARCH}" = "arm64" ] && ! command -v aarch64-linux-gnu-gcc >/dev/null 2>&1; then
  echo "Installing aarch64 cross-compile toolchain..."
  apt-get update -qq && apt-get install -y -qq gcc-aarch64-linux-gnu
fi
git fetch --tags --depth 100 origin 2>/dev/null || true
make \
  WHAT="cmd/kubeadm cmd/kubelet cmd/kubectl test/e2e/e2e.test vendor/github.com/onsi/ginkgo/v2/ginkgo" \
  KUBE_BUILD_PLATFORMS="linux/${LAMBDA_ARCH}"

# --- Transfer binaries to Lambda instance ---
lambda_remote mkdir -p /tmp/k8s-bins
lambda_rsync_to _output/local/bin/linux/${LAMBDA_ARCH}/{kubeadm,kubelet,kubectl} "ubuntu@${LAMBDA_INSTANCE_IP}:/tmp/k8s-bins/"
lambda_rsync_to _output/local/bin/linux/${LAMBDA_ARCH}/{e2e.test,ginkgo} "ubuntu@${LAMBDA_INSTANCE_IP}:/tmp/"

# --- Set up single-node k8s cluster with GPU support ---
# No CDI, no Docker, no extra labels needed for device-plugin.
lambda_remote bash -s < "${SCRIPT_DIR}/lib/setup-k8s-node.sh"

# --- Deploy NVIDIA device plugin and wait for GPU capacity ---
# Download manifest on Prow side (reliable network) and transfer to Lambda instance.
# This avoids TLS issues on some Lambda instances when fetching from raw.githubusercontent.com.
DEVICE_PLUGIN_MANIFEST="/tmp/nvidia-device-plugin.yml"
curl -sSfL -o "${DEVICE_PLUGIN_MANIFEST}" \
  https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/v0.17.1/deployments/static/nvidia-device-plugin.yml
lambda_rsync_to "${DEVICE_PLUGIN_MANIFEST}" "ubuntu@${LAMBDA_INSTANCE_IP}:/tmp/"

lambda_remote bash -s <<'EOF'
set -eux
export KUBECONFIG=$HOME/.kube/config
kubectl apply -f /tmp/nvidia-device-plugin.yml
kubectl -n kube-system rollout status daemonset/nvidia-device-plugin-daemonset --timeout=120s

# Wait for GPU capacity to appear on the node
for i in $(seq 1 30); do
  GPU_COUNT=$(kubectl get nodes -o jsonpath='{.items[0].status.capacity.nvidia\.com/gpu}' 2>/dev/null || echo "0")
  [ -z "${GPU_COUNT}" ] && GPU_COUNT="0"
  [ "${GPU_COUNT}" != "0" ] && break
  sleep 2
done
echo "GPUs detected: ${GPU_COUNT}"
if [ "${GPU_COUNT}" = "0" ]; then
  echo "ERROR: No GPUs detected"
  exit 1
fi
EOF

# --- Run GPU e2e tests ---
# Default skip pattern; jobs may override via the GINKGO_SKIP env var
# (e.g. the gh200 periodic skips the two [Feature:GPUDevicePlugin] tests
# whose container images are amd64-only).
GINKGO_SKIP="${GINKGO_SKIP:-\\[Flaky\\]}"
lambda_remote bash -s <<TESTEOF
set -eux
export KUBECONFIG=\$HOME/.kube/config
mkdir -p /tmp/gpu-test-artifacts
/tmp/ginkgo \
  -timeout=60m \
  -focus="\[Feature:GPUDevicePlugin\]" \
  -skip="${GINKGO_SKIP}" \
  -v \
  /tmp/e2e.test \
  -- \
  --provider=aws \
  --kubeconfig=\$KUBECONFIG \
  --report-dir=/tmp/gpu-test-artifacts \
  --minStartupPods=8
TESTEOF

# --- Collect artifacts ---
lambda_collect_artifacts /tmp/gpu-test-artifacts/
