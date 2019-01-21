// Package plugins implements Kubernetes plugins.
package plugins

// CreateInstall creates Kubernetes install script.
func CreateInstall() string {
	return installKubernetesAmazonLinux2Template
}

const installKubernetesAmazonLinux2Template = `

################################## install Kubernetes on Amazon Linux 2

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
crictl --version

sudo rm -rf /srv/kubernetes/
sudo mkdir -p /srv/kubernetes/

sudo rm -rf /etc/kubernetes/manifests/
sudo mkdir -p /etc/kubernetes/manifests/

sudo rm -rf /opt/cni/bin/
sudo mkdir -p /opt/cni/bin/

sudo rm -rf /etc/cni/net.d/
sudo mkdir -p /etc/cni/net.d/

##################################

`
