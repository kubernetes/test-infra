// Package plugins implements kubeadm plugins.
package plugins

import (
	"bytes"
	"text/template"
)

// CreateInstallStart creates kubeadm install init script.
func CreateInstallStart(ver string) (string, error) {
	tpl := template.Must(template.New("installStartKubeadmAmazonLinux2Template").Parse(installStartKubeadmAmazonLinux2Template))
	buf := bytes.NewBuffer(nil)
	kv := kubeadmInfo{Version: ver}
	if err := tpl.Execute(buf, kv); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type kubeadmInfo struct {
	Version string
}

// https://kubernetes.io/docs/setup/independent/install-kubeadm/
const installStartKubeadmAmazonLinux2Template = `

################################## install kubeadm on Amazon Linux 2

cat <<EOF > /tmp/kubernetes.repo
[kubernetes]
name=Kubernetes
baseurl=https://packages.cloud.google.com/yum/repos/kubernetes-el7-x86_64
enabled=1
gpgcheck=1
repo_gpgcheck=0
gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
exclude=kube*
EOF
sudo cp /tmp/kubernetes.repo /etc/yum.repos.d/kubernetes.repo

cat <<EOF > /tmp/k8s.conf
net.bridge.bridge-nf-call-ip6tables = 1
net.bridge.bridge-nf-call-iptables = 1
EOF
sudo cp /tmp/k8s.conf /etc/sysctl.d/k8s.conf
sudo sysctl --system
sudo sysctl net.bridge.bridge-nf-call-iptables=1

# Set SELinux in permissive mode (effectively disabling it)
setenforce 0
sudo sed -i 's/^SELINUX=enforcing$/SELINUX=permissive/' /etc/selinux/config

sudo yum install -y cri-tools ebtables kubernetes-cni socat iproute-tc

RELEASE=v{{ .Version }}

cd /usr/bin
sudo rm -f /usr/bin/{kubeadm,kubelet,kubectl}

sudo curl -L --remote-name-all https://storage.googleapis.com/kubernetes-release/release/${RELEASE}/bin/linux/amd64/{kubeadm,kubelet,kubectl}
sudo chmod +x {kubeadm,kubelet,kubectl}

curl -sSL "https://raw.githubusercontent.com/kubernetes/kubernetes/${RELEASE}/build/debs/kubelet.service" > /tmp/kubelet.service
cat /tmp/kubelet.service

# curl -sSL "https://raw.githubusercontent.com/kubernetes/kubernetes/${RELEASE}/build/debs/10-kubeadm.conf" > /tmp/10-kubeadm.conf
# sudo sed -i 's/cgroup-driver=cgroupfs/cgroup-driver=systemd/' /tmp/10-kubeadm.conf

# delete cni binary
# https://github.com/coreos/coreos-kubernetes/issues/874
cat << EOT > /tmp/10-kubeadm.conf
[Service]
Environment="KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=/etc/kubernetes/bootstrap-kubelet.conf --kubeconfig=/etc/kubernetes/kubelet.conf"
Environment="KUBELET_SYSTEM_PODS_ARGS=--pod-manifest-path=/etc/kubernetes/manifests --allow-privileged=true"
Environment="KUBELET_NETWORK_ARGS="
Environment="KUBELET_DNS_ARGS=--cluster-dns=10.96.0.10 --cluster-domain=cluster.local"
Environment="KUBELET_AUTHZ_ARGS=--authorization-mode=Webhook --client-ca-file=/etc/kubernetes/pki/ca.crt"
# Value should match Docker daemon settings.
# Defaults are "cgroupfs" for Debian/Ubuntu/OpenSUSE and "systemd" for Fedora/CentOS/RHEL
Environment="KUBELET_CGROUP_ARGS=--cgroup-driver=systemd"
Environment="KUBELET_CADVISOR_ARGS=--cadvisor-port=0"
Environment="KUBELET_CERTIFICATE_ARGS=--rotate-certificates=true"
ExecStart=
ExecStart=/usr/bin/kubelet __KUBELET_KUBECONFIG_ARGS __KUBELET_SYSTEM_PODS_ARGS __KUBELET_NETWORK_ARGS __KUBELET_DNS_ARGS __KUBELET_AUTHZ_ARGS __KUBELET_CGROUP_ARGS __KUBELET_CADVISOR_ARGS __KUBELET_CERTIFICATE_ARGS __KUBELET_EXTRA_ARGS
EOT
cat /tmp/10-kubeadm.conf
sed -i.bak 's|__KUBELET|\$KUBELET|g' /tmp/10-kubeadm.conf
cat /tmp/10-kubeadm.conf

sudo mkdir -p /etc/systemd/system/kubelet.service.d
sudo cp /tmp/kubelet.service /etc/systemd/system/kubelet.service
sudo cp /tmp/10-kubeadm.conf /etc/systemd/system/kubelet.service.d/10-kubeadm.conf

sudo systemctl daemon-reload
sudo systemctl cat kubelet.service
sudo systemctl enable kubelet && sudo systemctl restart kubelet
sudo systemctl status kubelet --full --no-pager || true
sudo journalctl --no-pager --output=cat -u kubelet

kubeadm version
kubelet --version
kubectl version --client
crictl --version

##################################

`
