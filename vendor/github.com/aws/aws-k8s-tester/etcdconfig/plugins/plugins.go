// Package plugins implements etcd plugins.
package plugins

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/blang/semver"
)

// CreateInstallScript returns the etcd install script for Linux.
// Inputs are either: "v3.2.12", "master", "123" (PR number).
func CreateInstallScript(ver string) (s string, err error) {
	v, err := semver.Make(ver)
	if err == nil {
		return createInstallRelase(etcdInfo{Version: v.String()})
	}
	if err != nil && strings.Contains(err.Error(), "Invalid character(s) found in major") && strings.HasPrefix(ver, "v") {
		ver = ver[1:]
		v, err = semver.Make(ver)
		if err == nil {
			return createInstallRelase(etcdInfo{Version: v.String()})
		}
	}

	_, perr := strconv.ParseInt(ver, 10, 64)
	isPR := perr == nil
	return createInstallGit(gitInfo{
		GitRepo:      "etcd",
		GitClonePath: "${GOPATH}/src/go.etcd.io",
		GitCloneURL:  "https://github.com/etcd-io/etcd.git",
		IsPR:         isPR,
		GitBranch:    ver,
		InstallScript: `./build
sudo cp ./bin/etcd /usr/local/bin/etcd
sudo cp ./bin/etcdctl /usr/local/bin/etcdctl

/usr/local/bin/etcd --version
ETCDCTL_API=3 /usr/local/bin/etcdctl version`,
	})
}

func createInstallRelase(g etcdInfo) (string, error) {
	tpl := template.Must(template.New("installRelease").Parse(installRelease))
	buf := bytes.NewBuffer(nil)
	if err := tpl.Execute(buf, g); err != nil {
		return "", err
	}
	return buf.String(), nil
}

type etcdInfo struct {
	Version string
}

const installRelease = `

################################## install etcd

sudo systemctl stop etcd.service || true

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

/usr/local/bin/etcd --version
ETCDCTL_API=3 /usr/local/bin/etcdctl version

##################################

`

func createInstallGit(g gitInfo) (string, error) {
	if g.IsPR {
		_, serr := strconv.ParseInt(g.GitBranch, 10, 64)
		if serr != nil {
			return "", fmt.Errorf("expected PR number, got %q (%v)", g.GitBranch, serr)
		}
	}
	tpl := template.Must(template.New("installGit").Parse(installGit))
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

const installGit = `

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
