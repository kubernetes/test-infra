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

# setup-nvkind-node.sh -- run on the GCE GPU VM via SSH.
# Installs Docker + NVIDIA toolkit config + Go + kind + nvkind + helm +
# kubectl, then creates a DRA-enabled nvkind cluster.
#
# Required env: NVKIND_CONFIG_PATH (kind config template already on the VM).
# Optional env (with defaults):
#   NVKIND_CLUSTER_NAME  nvkind-ci
#   NVKIND_K8S_VERSION   v1.34.3
#   NODE_LABELS          "nvidia.com/gpu.present=true feature.node.kubernetes.io/pci-10de.present=true"
#   GO_VERSION           1.24.2
#   KUBECTL_VERSION      $NVKIND_K8S_VERSION
#   NVKIND_DELETE_EXISTING  if "true", wipe+recreate if cluster exists

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

: "${NVKIND_CLUSTER_NAME:=nvkind-ci}"
: "${NVKIND_K8S_VERSION:=v1.34.3}"
: "${NVKIND_CONFIG_PATH:?NVKIND_CONFIG_PATH must be set}"
: "${NODE_LABELS:=nvidia.com/gpu.present=true feature.node.kubernetes.io/pci-10de.present=true}"
: "${GO_VERSION:=1.24.2}"
: "${KUBECTL_VERSION:=${NVKIND_K8S_VERSION}}"

export DEBIAN_FRONTEND=noninteractive

# Fail fast if no GPU.
if ! nvidia-smi -L 2>/dev/null | grep -q '^GPU '; then
  echo "ERROR: nvidia-smi reports no GPU on this host" >&2
  exit 1
fi
echo "Host sees: $(nvidia-smi -L | head -1)"

sudo apt-get update -qq
sudo apt-get install -y -qq ca-certificates curl gnupg make build-essential git jq

# Docker CE.
if ! command -v docker >/dev/null 2>&1; then
  sudo install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg |
    sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  sudo chmod a+r /etc/apt/keyrings/docker.gpg
  echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" |
    sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
  sudo apt-get update -qq
  sudo apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
fi
# Idempotent; covers the pre-installed-Docker case.
sudo usermod -aG docker "$(id -un)"

# nvidia-container-toolkit (DLVM ships it; stock Ubuntu does not).
if ! command -v nvidia-ctk >/dev/null 2>&1; then
  curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey |
    sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
  curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list |
    sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' |
    sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list > /dev/null
  sudo apt-get update -qq
  sudo apt-get install -y -qq nvidia-container-toolkit
fi

# Toolkit config. Restart docker only if daemon.json actually changed, so
# re-invocations don't nuke a running kind cluster.
before=$(sudo sha256sum /etc/docker/daemon.json 2>/dev/null | awk '{print $1}' || echo none)
sudo nvidia-ctk runtime configure --runtime=docker --set-as-default --cdi.enabled
sudo nvidia-ctk config --set accept-nvidia-visible-devices-as-volume-mounts=true --in-place
after=$(sudo sha256sum /etc/docker/daemon.json 2>/dev/null | awk '{print $1}' || echo none)
if [ "${before}" != "${after}" ]; then
  sudo systemctl restart docker
fi

# Smoke: docker can see the GPU.
sudo docker run --rm --runtime=nvidia -e NVIDIA_VISIBLE_DEVICES=all ubuntu:22.04 nvidia-smi -L

# Go, kubectl, helm.
if ! /usr/local/go/bin/go version 2>/dev/null | grep -q "${GO_VERSION}"; then
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz
  sudo rm -rf /usr/local/go
  sudo tar -C /usr/local -xzf /tmp/go.tgz
fi
export PATH="/usr/local/go/bin:${HOME}/go/bin:${PATH}"

if ! kubectl version --client 2>/dev/null | grep -q "${KUBECTL_VERSION}"; then
  curl -fsSLo /tmp/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl"
  sudo install /tmp/kubectl /usr/local/bin/kubectl
fi

if ! command -v helm >/dev/null 2>&1; then
  curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
fi

# kind (pinned to the version nvkind vendors) and nvkind. nvkind has no
# tags; pin a commit SHA. Bump with a reviewable diff after re-validating.
: "${NVKIND_SHA:=8bce71ec58cf12b4003758eb4e49adac53cc40f2}"
go install sigs.k8s.io/kind@v0.31.0
go install "github.com/NVIDIA/nvkind/cmd/nvkind@${NVKIND_SHA}"

# `sg docker -c` avoids re-login after usermod -aG docker.
if sg docker -c "kind get clusters" | grep -qx "${NVKIND_CLUSTER_NAME}"; then
  if [ "${NVKIND_DELETE_EXISTING:-false}" = "true" ]; then
    sg docker -c "kind delete cluster --name=${NVKIND_CLUSTER_NAME}"
  else
    echo "ERROR: cluster ${NVKIND_CLUSTER_NAME} exists; set NVKIND_DELETE_EXISTING=true to recreate" >&2
    exit 1
  fi
fi

sg docker -c "nvkind cluster create \
  --name ${NVKIND_CLUSTER_NAME} \
  --image kindest/node:${NVKIND_K8S_VERSION} \
  --config-template ${NVKIND_CONFIG_PATH}"

kubectl --context "kind-${NVKIND_CLUSTER_NAME}" wait \
  --for=condition=Ready nodes --all --timeout=300s

for node in $(kubectl --context "kind-${NVKIND_CLUSTER_NAME}" get nodes -l '!node-role.kubernetes.io/control-plane' -o name); do
  # shellcheck disable=SC2086
  kubectl --context "kind-${NVKIND_CLUSTER_NAME}" label $node $NODE_LABELS --overwrite
done

sg docker -c "nvkind cluster print-gpus --name ${NVKIND_CLUSTER_NAME}"
echo "nvkind cluster ${NVKIND_CLUSTER_NAME} ready"
