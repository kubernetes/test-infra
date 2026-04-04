#!/bin/bash
# setup-cluster.sh — runs on the Lambda GPU instance via SSH.
# Sets up a single-node kubeadm cluster with NVIDIA GPU support.
# Expects k8s binaries (kubeadm, kubelet, kubectl) already in /tmp/.
set -euxo pipefail

sudo apt-get update -qq
sudo apt-get install -y -qq \
  build-essential pkg-config libseccomp-dev libseccomp2 \
  iptables iproute2 conntrack ebtables kmod socat ethtool \
  jq rsync psmisc curl wget

# Install CNI plugins
CNI_VERSION=$(curl -s https://api.github.com/repos/containernetworking/plugins/releases/latest | grep tag_name | cut -d'"' -f4)
sudo mkdir -p /opt/cni/bin
curl -sL "https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/cni-plugins-linux-amd64-${CNI_VERSION}.tgz" \
  | sudo tar -C /opt/cni/bin -xz

# Configure CNI networking
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
    {
      "type": "portmap",
      "capabilities": {"portMappings": true}
    }
  ]
}
EOF

# Configure containerd with NVIDIA runtime
sudo mkdir -p /etc/containerd
sudo containerd config default | sudo tee /etc/containerd/config.toml > /dev/null
sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' /etc/containerd/config.toml
sudo nvidia-ctk runtime configure --runtime=containerd --set-as-default
sudo systemctl restart containerd

# Enable IP forwarding and bridge netfilter
sudo modprobe br_netfilter
echo 1 | sudo tee /proc/sys/net/bridge/bridge-nf-call-iptables
echo 1 | sudo tee /proc/sys/net/ipv4/ip_forward
sudo tee /etc/sysctl.d/99-kubernetes.conf > /dev/null <<'EOF'
net.bridge.bridge-nf-call-iptables = 1
net.ipv4.ip_forward = 1
EOF
sudo sysctl --system
sudo swapoff -a || true

# Install k8s binaries
sudo cp /tmp/kubeadm /tmp/kubelet /tmp/kubectl /usr/local/bin/
sudo chmod +x /usr/local/bin/{kubeadm,kubelet,kubectl}

# Create kubelet systemd service
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

# Initialize cluster
sudo kubeadm init \
  --pod-network-cidr=10.88.0.0/16 \
  --cri-socket=unix:///run/containerd/containerd.sock \
  --ignore-preflight-errors=NumCPU,Mem,FileContent--proc-sys-net-bridge-bridge-nf-call-iptables,SystemVerification

# Configure kubectl
mkdir -p "$HOME/.kube"
sudo cp /etc/kubernetes/admin.conf "$HOME/.kube/config"
sudo chown "$(id -u):$(id -g)" "$HOME/.kube/config"

# Allow pods on control-plane
kubectl taint nodes --all node-role.kubernetes.io/control-plane-

# Fix pod networking
sudo iptables -I FORWARD -i cni0 -j ACCEPT
sudo iptables -I FORWARD -o cni0 -j ACCEPT
sudo iptables -I FORWARD -i cni0 -o cni0 -j ACCEPT
sudo iptables -t nat -A POSTROUTING -s 10.88.0.0/16 ! -o cni0 -j MASQUERADE

# Deploy NVIDIA device plugin
kubectl create -f https://raw.githubusercontent.com/NVIDIA/k8s-device-plugin/v0.17.1/deployments/static/nvidia-device-plugin.yml
kubectl -n kube-system rollout status daemonset/nvidia-device-plugin-daemonset --timeout=120s

# Verify GPU is visible
for i in $(seq 1 30); do
  GPU_COUNT=$(kubectl get nodes -o jsonpath='{.items[0].status.capacity.nvidia\.com/gpu}' 2>/dev/null || true)
  [ -z "${GPU_COUNT}" ] && GPU_COUNT="0"
  [ "${GPU_COUNT}" != "0" ] && break
  sleep 2
done
echo "GPUs detected: ${GPU_COUNT}"
if [ "${GPU_COUNT}" = "0" ]; then
  echo "ERROR: No GPUs detected"
  exit 1
fi
