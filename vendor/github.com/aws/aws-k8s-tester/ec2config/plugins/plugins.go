package plugins

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/template"

	etcdplugin "github.com/aws/aws-k8s-tester/etcdconfig/plugins"
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
	"update-amazon-linux-2":         1,
	"update-ubuntu":                 2,
	"set-env-aws-cred":              3, // TODO: use instance role instead
	"mount-aws-cred":                4, // TODO: use instance role instead
	"install-go":                    5,
	"install-csi":                   6,
	"install-etcd":                  7,
	"install-aws-k8s-tester":        8,
	"install-wrk":                   9,
	"install-alb":                   10,
	"install-kubeadm-ubuntu":        11,
	"install-docker-amazon-linux-2": 12,
	"install-docker-ubuntu":         13,
}

func convertToScript(userName, plugin string) (script, error) {
	switch {
	case plugin == "update-amazon-linux-2":
		return script{key: "update-amazon-linux-2", data: updateAmazonLinux2}, nil

	case plugin == "update-ubuntu":
		return script{key: "update-ubuntu", data: updateUbuntu}, nil

	case strings.HasPrefix(plugin, "set-env-aws-cred-"):
		// TODO: use instance role instead
		env := strings.Replace(plugin, "set-env-aws-cred-", "", -1)
		if os.Getenv(env) == "" {
			return script{}, fmt.Errorf("%q is not defined", env)
		}
		d, derr := ioutil.ReadFile(os.Getenv(env))
		if derr != nil {
			return script{}, derr
		}
		lines := strings.Split(string(d), "\n")
		prevDefault := false
		accessKey, accessSecret := "", ""
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if line == "[default]" {
				prevDefault = true
				continue
			}
			if prevDefault {
				if strings.HasPrefix(line, "aws_access_key_id = ") {
					accessKey = strings.Replace(line, "aws_access_key_id = ", "", -1)
				}
				if strings.HasPrefix(line, "aws_secret_access_key = ") {
					accessSecret = strings.Replace(line, "aws_secret_access_key = ", "", -1)
					break
				}
			}
		}
		return script{
			key: "set-env-aws-cred",
			data: fmt.Sprintf(`echo "export AWS_ACCESS_KEY_ID=%s" >> /home/%s/.bashrc
echo "export AWS_SECRET_ACCESS_KEY=%s" >> /home/%s/.bashrc
`, accessKey, userName, accessSecret, userName),
		}, nil

	case strings.HasPrefix(plugin, "mount-aws-cred-"):
		// TODO: use instance role instead
		env := strings.Replace(plugin, "mount-aws-cred-", "", -1)
		if os.Getenv(env) == "" {
			return script{}, fmt.Errorf("%q is not defined", env)
		}
		d, derr := ioutil.ReadFile(os.Getenv(env))
		if derr != nil {
			return script{}, derr
		}
		return script{
			key: "mount-aws-cred",
			data: fmt.Sprintf(`
mkdir -p /home/%s/.aws/

cat << EOT > /home/%s/.aws/credentials
%s
EOT`, userName, userName, string(d)),
		}, nil

	case plugin == "install-go1.11.2":
		s, err := createInstallGo(goInfo{
			UserName:  userName,
			GoVersion: "1.11.2",
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

	case plugin == "install-kubeadm-ubuntu":
		return script{
			key:  plugin,
			data: installKubeadmnUbuntu,
		}, nil

	case plugin == "install-docker-amazon-linux-2":
		return script{
			key:  plugin,
			data: installDockerAmazonLinux2,
		}, nil

	case plugin == "install-docker-ubuntu":
		return script{
			key:  plugin,
			data: installDockerUbuntu,
		}, nil
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
mkdir -p ${GOPATH}/src/github.com/kubernetes-sigs
mkdir -p ${GOPATH}/src/k8s.io
mkdir -p ${GOPATH}/src/sigs.k8s.io

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

const installKubeadmnUbuntu = `

################################## install kubeadm on Ubuntu

cd ${HOME}

sudo apt-get update -y && sudo apt-get install -y apt-transport-https curl
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -

cat <<EOF >/tmp/kubernetes.list
deb https://apt.kubernetes.io/ kubernetes-$(lsb_release -cs) main
EOF

sudo cp /tmp/kubernetes.list /etc/apt/sources.list.d/kubernetes.list

sudo apt-get update -y
sudo apt-get install -y kubelet kubeadm kubectl || true
sudo apt-mark hold kubelet kubeadm kubectl || true

sudo systemctl enable kubelet
sudo systemctl start kubelet

sudo journalctl --no-pager --output=cat -u kubelet

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

const installDockerUbuntu = `

################################## install Docker on Ubuntu
sudo apt update -y
sudo apt install -y apt-transport-https ca-certificates curl software-properties-common

curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable"

sudo apt update -y
apt-cache policy docker-ce || true
sudo apt install -y docker-ce

sudo systemctl start docker || true
sudo systemctl status docker --full --no-pager || true
sudo usermod -aG docker ubuntu || true

# su - ubuntu
# or logout and login to use docker without 'sudo'

id -nG
sudo docker version
sudo docker info
##################################

`

// https://docs.aws.amazon.com/AmazonECS/latest/developerguide/docker-basics.html
const installDockerAmazonLinux2 = `

################################## install Docker on Amazon Linux 2
sudo yum update -y
sudo yum install -y docker

sudo systemctl start docker || true
sudo systemctl status docker --full --no-pager || true
sudo usermod -aG docker ec2-user || true

# su - ec2-user
# or logout and login to use docker without 'sudo'

id -nG
sudo docker version
sudo docker info
##################################

`
