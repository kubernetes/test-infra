// Package plugins implements kubeadm plugins.
package plugins

import (
	"bytes"
	"text/template"
)

// CreateInstall creates kubeadm install script.
func CreateInstall(ver string) (string, error) {
	tpl := template.Must(template.New("installKubeadmAmazonLinux2Template").Parse(installKubeadmAmazonLinux2Template))
	buf := bytes.NewBuffer(nil)
	kv := kubeadmInfo{Version: ver, KubeletPath: "/usr/bin/kubelet"}
	if err := tpl.Execute(buf, kv); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type kubeadmInfo struct {
	Version     string
	KubeletPath string
}

// https://kubernetes.io/docs/setup/independent/install-kubeadm/
const installKubeadmAmazonLinux2Template = `

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

sudo systemctl stop kubelet.service || true
sudo mkdir -p /var/lib/kubelet/

rm -f /tmp/kubelet.service
cat <<EOF > /tmp/kubelet.service
[Unit]
Description=kubelet
Documentation=http://kubernetes.io/docs/
After=docker.service

[Service]
EnvironmentFile=/etc/sysconfig/kubelet
ExecStart={{ .KubeletPath }} "\$KUBELET_FLAGS"
Restart=always
RestartSec=2s
StartLimitInterval=0
KillMode=process
User=root

[Install]
WantedBy=multi-user.target
EOF
cat /tmp/kubelet.service

sudo mkdir -p /etc/systemd/system/kubelet.service.d
sudo cp /tmp/kubelet.service /etc/systemd/system/kubelet.service

sudo systemctl daemon-reload
sudo systemctl cat kubelet.service

##################################

`
