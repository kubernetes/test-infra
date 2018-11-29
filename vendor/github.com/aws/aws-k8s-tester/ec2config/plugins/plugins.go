// Package plugins defines various plugins to install on EC2 creation,
// using init scripts or EC2 user data.
package plugins

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/template"

	etcdplugin "github.com/aws/aws-k8s-tester/etcdconfig/plugins"
	kubeadmplugin "github.com/aws/aws-k8s-tester/kubeadmconfig/plugins"
)

// headerBash is the bash script header.
const headerBash = `#!/usr/bin/env bash`

// READY is appended on init script complete.
const READY = "AWS_K8S_TESTER_EC2_PLUGIN_READY"

type script struct {
	key  string
	data string
}

type scripts []script

func (ss scripts) Len() int           { return len(ss) }
func (ss scripts) Swap(i, j int)      { ss[i], ss[j] = ss[j], ss[i] }
func (ss scripts) Less(i, j int) bool { return keyPriorities[ss[i].key] < keyPriorities[ss[j].key] }

var keyPriorities = map[string]int{ // in the order of:
	"update-amazon-linux-2":                1,
	"update-ubuntu":                        2,
	"install-go":                           3,
	"install-csi":                          4,
	"install-etcd":                         5,
	"install-aws-k8s-tester":               6,
	"install-wrk":                          7,
	"install-alb":                          8,
	"install-start-docker-amazon-linux-2":  9,
	"install-start-kubeadm-amazon-linux-2": 10,
}

func convertToScript(userName, plugin string) (script, error) {
	switch {
	case plugin == "update-amazon-linux-2":
		return script{key: "update-amazon-linux-2", data: updateAmazonLinux2}, nil

	case plugin == "update-ubuntu":
		return script{key: "update-ubuntu", data: updateUbuntu}, nil

	case strings.HasPrefix(plugin, "install-go-"):
		goVer := strings.Replace(plugin, "install-go-", "", -1)
		s, err := createInstallGo(goInfo{
			UserName:  userName,
			GoVersion: goVer,
		})
		if err != nil {
			return script{}, err
		}
		return script{
			key:  "install-go",
			data: s,
		}, nil

	case strings.HasPrefix(plugin, "install-csi-"):
		gitBranch := strings.Replace(plugin, "install-csi-", "", -1)
		_, perr := strconv.ParseInt(gitBranch, 10, 64)
		isPR := perr == nil
		s, err := createInstallGit(gitInfo{
			GitRepo:       "aws-ebs-csi-driver",
			GitClonePath:  "${GOPATH}/src/github.com/kubernetes-sigs",
			GitCloneURL:   "https://github.com/kubernetes-sigs/aws-ebs-csi-driver.git",
			IsPR:          isPR,
			GitBranch:     gitBranch,
			InstallScript: `make aws-ebs-csi-driver && sudo cp ./bin/aws-ebs-csi-driver /usr/local/bin/aws-ebs-csi-driver`,
		})
		if err != nil {
			return script{}, err
		}
		return script{key: "install-csi", data: s}, nil

	case strings.HasPrefix(plugin, "install-etcd-"):
		id := strings.Replace(plugin, "install-etcd-", "", -1)
		s, err := etcdplugin.CreateInstallScript(id)
		if err != nil {
			return script{}, err
		}
		return script{key: "install-etcd", data: s}, nil

	case plugin == "install-aws-k8s-tester":
		s, err := createInstallGit(gitInfo{
			GitRepo:       "aws-k8s-tester",
			GitClonePath:  "${GOPATH}/src/github.com/aws",
			GitCloneURL:   "https://github.com/aws/aws-k8s-tester.git",
			IsPR:          false,
			GitBranch:     "master",
			InstallScript: `go build -v ./cmd/aws-k8s-tester && sudo cp ./aws-k8s-tester /usr/local/bin/aws-k8s-tester`,
		})
		if err != nil {
			return script{}, err
		}
		return script{key: "install-aws-k8s-tester", data: s}, nil

	case plugin == "install-wrk":
		return script{
			key:  plugin,
			data: installWrk,
		}, nil

	case strings.HasPrefix(plugin, "install-alb-"):
		gitBranch := strings.Replace(plugin, "install-alb-", "", -1)
		_, perr := strconv.ParseInt(gitBranch, 10, 64)
		isPR := perr == nil
		s, err := createInstallGit(gitInfo{
			GitRepo:      "aws-alb-ingress-controller",
			GitClonePath: "${GOPATH}/src/github.com/kubernetes-sigs",
			GitCloneURL:  "https://github.com/kubernetes-sigs/aws-alb-ingress-controller.git",
			IsPR:         isPR,
			GitBranch:    gitBranch,
			InstallScript: `GO111MODULE=on go mod vendor -v
make server
			`,
		})
		if err != nil {
			return script{}, err
		}
		return script{key: "install-alb", data: s}, nil

	case plugin == "install-start-docker-amazon-linux-2":
		return script{
			key:  plugin,
			data: installStartDockerAmazonLinux2,
		}, nil

	case strings.HasPrefix(plugin, "install-start-kubeadm-amazon-linux-2-"):
		id := strings.Replace(plugin, "install-start-kubeadm-amazon-linux-2-", "", -1)
		s, err := kubeadmplugin.CreateInstallStart(id)
		if err != nil {
			return script{}, err
		}
		return script{key: "install-start-kubeadm-amazon-linux-2", data: s}, nil
	}

	return script{}, fmt.Errorf("unknown plugin %q", plugin)
}

// Create returns the plugin.
func Create(userName string, plugins []string) (data string, err error) {
	sts := make([]script, 0, len(plugins))
	for _, plugin := range plugins {
		if plugin == "update-ubuntu" {
			if userName != "ubuntu" {
				return "", fmt.Errorf("'update-ubuntu' requires 'ubuntu' user name, got %q", userName)
			}
		}
		script, err := convertToScript(userName, plugin)
		if err != nil {
			return "", err
		}
		sts = append(sts, script)
	}
	sort.Sort(scripts(sts))

	data = headerBash
	for _, s := range sts {
		data += s.data
	}
	data += fmt.Sprintf("\n\necho %s\n\n", READY)
	return data, nil
}

const updateAmazonLinux2 = `

################################## update Amazon Linux 2

export HOME=/home/ec2-user
export GOPATH=/home/ec2-user/go

sudo yum update -y \
  && sudo yum install -y \
  gcc \
  zlib-devel \
  openssl-devel \
  ncurses-devel \
  git \
  wget \
  jq \
  tar \
  curl \
  unzip \
  screen \
  mercurial

##################################

`
const updateUbuntu = `

################################## update Ubuntu

export HOME=/home/ubuntu
export GOPATH=/home/ubuntu/go

apt-get -y update \
  && apt-get -y install \
  build-essential \
  gcc \
  jq \
  file \
  apt-utils \
  pkg-config \
  software-properties-common \
  apt-transport-https \
  ca-certificates \
  libssl-dev \
  gnupg2 \
  sudo \
  bash \
  curl \
  wget \
  tar \
  git \
  screen \
  mercurial \
  openssh-client \
  rsync \
  unzip \
  wget \
  xz-utils \
  zip \
  zlib1g-dev \
  lsb-release \
  python3 \
  python3-pip \
  python3-setuptools \
  && apt-get clean \
  && pip3 install awscli --no-cache-dir --upgrade \
  && which aws && aws --version \
  && apt-get -y install \
  python \
  python-dev \
  python-openssl \
  python-pip \
  && pip install --upgrade pip setuptools wheel

##################################

`

func createInstallGo(g goInfo) (string, error) {
	tpl := template.Must(template.New("installGoTemplate").Parse(installGoTemplate))
	buf := bytes.NewBuffer(nil)
	if err := tpl.Execute(buf, g); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type goInfo struct {
	UserName  string
	GoVersion string
}

const installGoTemplate = `

################################## install Go

export HOME=/home/{{ .UserName }}
export GOPATH=/home/{{ .UserName }}/go

GO_VERSION={{ .GoVersion }}
GOOGLE_URL=https://storage.googleapis.com/golang
DOWNLOAD_URL=${GOOGLE_URL}

sudo curl -s ${DOWNLOAD_URL}/go$GO_VERSION.linux-amd64.tar.gz | sudo tar -v -C /usr/local/ -xz

mkdir -p ${GOPATH}/bin/
mkdir -p ${GOPATH}/src/

if grep -q GOPATH "${HOME}/.bashrc"; then
  echo "bashrc already has GOPATH";
else
  echo "adding GOPATH to bashrc";
  echo "export GOPATH=${HOME}/go" >> ${HOME}/.bashrc;
  PATH_VAR=$PATH":/usr/local/go/bin:${HOME}/go/bin";
  echo "export PATH=$(echo $PATH_VAR)" >> ${HOME}/.bashrc;
  source ${HOME}/.bashrc;
fi

source ${HOME}/.bashrc
export PATH=$PATH:/usr/local/go/bin:${HOME}/go/bin

sudo echo PATH=${PATH} > /etc/environment
sudo echo GOPATH=/home/{{ .UserName }}/go >> /etc/environment
echo "source /etc/environment" >> ${HOME}/.bashrc;

go version

##################################

`

func createInstallEtcd(g etcdInfo) (string, error) {
	tpl := template.Must(template.New("installEtcdTemplate").Parse(installEtcdTemplate))
	buf := bytes.NewBuffer(nil)
	if err := tpl.Execute(buf, g); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type etcdInfo struct {
	Version string
}

const installEtcdTemplate = `

################################## install etcd

ETCD_VER=v{{ .Version }}

# choose either URL
GOOGLE_URL=https://storage.googleapis.com/etcd
GITHUB_URL=https://github.com/etcd-io/etcd/releases/download
DOWNLOAD_URL=${GOOGLE_URL}

rm -f /tmp/etcd-${ETCD_VER}-linux-amd64.tar.gz
rm -rf /tmp/etcd-download-test && mkdir -p /tmp/etcd-download-test

curl -L ${DOWNLOAD_URL}/${ETCD_VER}/etcd-${ETCD_VER}-linux-amd64.tar.gz -o /tmp/etcd-${ETCD_VER}-linux-amd64.tar.gz
tar xzvf /tmp/etcd-${ETCD_VER}-linux-amd64.tar.gz -C /tmp/etcd-download-test --strip-components=1
rm -f /tmp/etcd-${ETCD_VER}-linux-amd64.tar.gz

sudo cp /tmp/etcd-download-test/etcd /usr/local/bin/etcd
sudo cp /tmp/etcd-download-test/etcdctl /usr/local/bin/etcdctl

etcd --version
ETCDCTL_API=3 etcdctl version

##################################

`

const installWrk = `

################################## install wrk

cd ${HOME}

RETRIES=10
DELAY=10
COUNT=1
while [[ ${COUNT} -lt ${RETRIES} ]]; do
  rm -rf ./wrk
  git clone https://github.com/wg/wrk.git
  if [[ $? -eq 0 ]]; then
    RETRIES=0
    echo "Successfully git cloned!"
    break
  fi
  let COUNT=${COUNT}+1
  sleep ${DELAY}
done

cd ./wrk \
  && make all \
  && sudo cp ./wrk /usr/local/bin/wrk \
  && cd .. \
  && rm -rf ./wrk \
  && wrk --version || true && which wrk

##################################

`

func createInstallGit(g gitInfo) (string, error) {
	if g.IsPR {
		_, serr := strconv.ParseInt(g.GitBranch, 10, 64)
		if serr != nil {
			return "", fmt.Errorf("expected PR number, got %q (%v)", g.GitBranch, serr)
		}
	}
	tpl := template.Must(template.New("installGitTemplate").Parse(installGitTemplate))
	buf := bytes.NewBuffer(nil)
	if err := tpl.Execute(buf, g); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type gitInfo struct {
	GitRepo      string
	GitClonePath string
	GitCloneURL  string
	IsPR         bool

	// GitBranch name or PR number
	GitBranch string

	InstallScript string
}

const installGitTemplate = `

################################## install {{ .GitRepo }} via git

mkdir -p {{ .GitClonePath }}/
cd {{ .GitClonePath }}/

RETRIES=10
DELAY=10
COUNT=1
while [[ ${COUNT} -lt ${RETRIES} ]]; do
  rm -rf ./{{ .GitRepo }}
  git clone {{ .GitCloneURL }}
  if [[ $? -eq 0 ]]; then
    RETRIES=0
    echo "Successfully git cloned!"
    break
  fi
  let COUNT=${COUNT}+1
  sleep ${DELAY}
done

cd {{ .GitClonePath }}/{{ .GitRepo }}

{{ if .IsPR }}echo 'git fetching:' pull/{{ .GitBranch }}/head 'to test branch'
git fetch origin pull/{{ .GitBranch }}/head:test
git checkout test
{{ else }}
git checkout origin/{{ .GitBranch }}
git checkout -B {{ .GitBranch }}
{{ end }}

git remote -v
git branch
git log --pretty=oneline -5

pwd
{{ .InstallScript }}

##################################

`

// https://docs.aws.amazon.com/AmazonECS/latest/developerguide/docker-basics.html
// https://kubernetes.io/docs/setup/cri/#docker
const installStartDockerAmazonLinux2 = `

################################## install Docker on Amazon Linux 2

sudo yum update -y
sudo yum install -y docker
sudo yum install -y yum-utils device-mapper-persistent-data lvm2

sudo yum-config-manager \
  --add-repo \
  https://download.docker.com/linux/centos/docker-ce.repo

sudo yum update && sudo yum install -y docker-ce-18.06.1.ce
sudo mkdir -p /etc/docker

cat > /etc/docker/daemon.json <<EOF
{
  "exec-opts": ["native.cgroupdriver=systemd"],
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "100m"
  },
  "storage-driver": "overlay2",
  "storage-opts": [
    "overlay2.override_kernel_check=true"
  ]
}
EOF
mkdir -p /etc/systemd/system/docker.service.d

sudo systemctl daemon-reload
sudo systemctl enable docker || true
sudo systemctl start docker || true
sudo systemctl restart docker || true

sudo systemctl status docker --full --no-pager || true
sudo usermod -aG docker ec2-user || true

# su - ec2-user
# or logout and login to use docker without 'sudo'
id -nG
sudo docker version
sudo docker info

##################################

`
