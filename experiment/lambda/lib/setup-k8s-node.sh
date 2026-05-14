#!/bin/bash
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

# setup-k8s-node.sh -- generic single-node kubeadm cluster setup for Lambda
# GPU instances. Expects k8s binaries in /tmp/k8s-bins/.
#
# Base: system deps, CNI, containerd with NVIDIA runtime, kubeadm cluster.
# Optional (controlled by env vars):
#   ENABLE_CDI=true               -- enable CDI in containerd (required for DRA)
#   ENABLE_DOCKER=true            -- install Docker (needed by harnesses that run in containers)
#   NODE_LABELS="k=v,..."         -- additional node labels (e.g., nvidia.com/gpu.present=true)
#   KUBEADM_FEATURE_GATES="K=v"   -- feature gates for API server, scheduler, controller-manager, kubelet
#
# Does NOT install any GPU driver/plugin -- callers handle that.
set -euxo pipefail

ENABLE_CDI="${ENABLE_CDI:-false}"
ENABLE_DOCKER="${ENABLE_DOCKER:-false}"
NODE_LABELS="${NODE_LABELS:-}"
K8S_BINS_DIR="${K8S_BINS_DIR:-/tmp/k8s-bins}"
KUBEADM_FEATURE_GATES="${KUBEADM_FEATURE_GATES:-}"

# --- System dependencies ---
sudo apt-get update -qq
sudo apt-get install -y -qq \
  build-essential pkg-config libseccomp-dev libseccomp2 \
  iptables iproute2 conntrack ebtables kmod socat ethtool \
  jq rsync psmisc curl wget git

# --- Detect architecture ---
ARCH=$(uname -m | sed -e 's,x86_64,amd64,' -e 's,aarch64,arm64,')

# --- CNI plugins ---
CNI_VERSION=$(curl -s https://api.github.com/repos/containernetworking/plugins/releases/latest \
  | grep tag_name | cut -d'"' -f4)
sudo mkdir -p /opt/cni/bin
curl -sL "https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-linux-${ARCH}-${CNI_VERSION}.tgz" \
  | sudo tar -C /opt/cni/bin -xz

# --- CNI networking ---
sudo mkdir -p /etc/cni/net.d
sudo tee /etc/cni/net.d/10-containerd-net.conflist > /dev/null <<'EOF'
{
  "cniVersion": "1.0.0",
  "name": "containerd-net",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isGateway": true,
      "ipMasq": true,
      "promiscMode": true,
      "ipam": {
        "type": "host-local",
        "ranges": [[{"subnet": "10.88.0.0/16"}]],
        "routes": [{"dst": "0.0.0.0/0"}]
      }
    },
    {"type": "portmap", "capabilities": {"portMappings": true}}
  ]
}
EOF

# --- Containerd with NVIDIA runtime ---
sudo mkdir -p /etc/containerd
sudo containerd config default | sudo tee /etc/containerd/config.toml > /dev/null
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' /etc/containerd/config.toml
sudo nvidia-ctk runtime configure --runtime=containerd --set-as-default
if [ "${ENABLE_CDI}" = "true" ]; then
  sudo sed -i 's/enable_cdi = false/enable_cdi = true/g' /etc/containerd/config.toml
fi
sudo systemctl restart containerd

# --- Networking ---
sudo modprobe br_netfilter
echo 1 | sudo tee /proc/sys/net/bridge/bridge-nf-call-iptables
echo 1 | sudo tee /proc/sys/net/ipv4/ip_forward
sudo tee /etc/sysctl.d/99-kubernetes.conf > /dev/null <<'EOF'
net.bridge.bridge-nf-call-iptables = 1
net.ipv4.ip_forward = 1
EOF
sudo sysctl --system
sudo swapoff -a || true

# --- K8s binaries ---
sudo cp "${K8S_BINS_DIR}"/{kubeadm,kubelet,kubectl} /usr/local/bin/
sudo chmod +x /usr/local/bin/{kubeadm,kubelet,kubectl}

# --- Kubelet systemd service ---
sudo tee /etc/systemd/system/kubelet.service > /dev/null <<'EOF'
[Unit]
Description=kubelet: The Kubernetes Node Agent
Wants=network-online.target
After=network-online.target
[Service]
ExecStart=/usr/local/bin/kubelet
Restart=always
StartLimitInterval=0
RestartSec=10
[Install]
WantedBy=multi-user.target
EOF

sudo mkdir -p /etc/systemd/system/kubelet.service.d
sudo tee /etc/systemd/system/kubelet.service.d/10-kubeadm.conf > /dev/null <<'EOF'
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_CONFIG_ARGS=--config=/var/lib/kubelet/config.yaml"
EnvironmentFile=-/var/lib/kubelet/kubeadm-flags.env
ExecStart=
ExecStart=/usr/local/bin/kubelet $KUBELET_KUBECONFIG_ARGS $KUBELET_CONFIG_ARGS $KUBELET_EXTRA_ARGS
EOF

sudo systemctl daemon-reload
sudo systemctl enable kubelet

# --- kubeadm init ---
if [ -n "${KUBEADM_FEATURE_GATES}" ]; then
  # Use a config file to pass feature gates to all control plane components.
  # Config file is required because kubeadm CLI --feature-gates only controls
  # kubeadm's own gates, not apiserver/scheduler/controller-manager/kubelet.
  KUBEADM_CONFIG=/tmp/kubeadm-config.yaml
  cat > "${KUBEADM_CONFIG}" <<KCEOF
apiVersion: kubeadm.k8s.io/v1beta4
kind: InitConfiguration
nodeRegistration:
  criSocket: unix:///run/containerd/containerd.sock
  ignorePreflightErrors:
    - NumCPU
    - Mem
    - FileContent--proc-sys-net-bridge-bridge-nf-call-iptables
    - SystemVerification
    - KubeletVersion
---
apiVersion: kubeadm.k8s.io/v1beta4
kind: ClusterConfiguration
networking:
  podSubnet: 10.88.0.0/16
apiServer:
  extraArgs:
    - name: feature-gates
      value: ${KUBEADM_FEATURE_GATES}
scheduler:
  extraArgs:
    - name: feature-gates
      value: ${KUBEADM_FEATURE_GATES}
controllerManager:
  extraArgs:
    - name: feature-gates
      value: ${KUBEADM_FEATURE_GATES}
---
apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
featureGates:
KCEOF
  # Append each feature gate as a YAML key-value pair under featureGates.
  IFS=',' read -ra FG_PAIRS <<< "${KUBEADM_FEATURE_GATES}"
  for pair in "${FG_PAIRS[@]}"; do
    echo "  ${pair%%=*}: ${pair#*=}" >> "${KUBEADM_CONFIG}"
  done
  echo "kubeadm config with feature gates:"
  cat "${KUBEADM_CONFIG}"
  sudo kubeadm init --config "${KUBEADM_CONFIG}"
else
  sudo kubeadm init \
    --pod-network-cidr=10.88.0.0/16 \
    --cri-socket=unix:///run/containerd/containerd.sock \
    --ignore-preflight-errors=NumCPU,Mem,FileContent--proc-sys-net-bridge-bridge-nf-call-iptables,SystemVerification,KubeletVersion
fi

mkdir -p "$HOME/.kube"
sudo cp /etc/kubernetes/admin.conf "$HOME/.kube/config"
sudo chown "$(id -u):$(id -g)" "$HOME/.kube/config"

# --- Allow scheduling on control-plane ---
kubectl taint nodes --all node-role.kubernetes.io/control-plane-

# --- Pod networking fix ---
sudo iptables -I FORWARD -i cni0 -j ACCEPT
sudo iptables -I FORWARD -o cni0 -j ACCEPT
sudo iptables -I FORWARD -i cni0 -o cni0 -j ACCEPT
sudo iptables -t nat -A POSTROUTING -s 10.88.0.0/16 ! -o cni0 -j MASQUERADE

# --- Optional: node labels ---
if [ -n "${NODE_LABELS}" ]; then
  NODE_NAME=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')
  IFS=',' read -ra LABELS <<< "${NODE_LABELS}"
  for label in "${LABELS[@]}"; do
    kubectl label node "${NODE_NAME}" "${label}"
  done
fi

# --- Optional: Docker ---
if [ "${ENABLE_DOCKER}" = "true" ]; then
  curl -fsSL https://get.docker.com | sudo sh
  sudo usermod -aG docker ubuntu
fi

# --- Verify GPU ---
if ! nvidia-smi -L; then
  echo "ERROR: nvidia-smi failed -- no GPU visible"
  exit 1
fi
echo "GPU hardware verified: $(nvidia-smi -L | head -1)"

echo "=== Lambda node setup complete ==="
