#!/usr/bin/env bash
# Copyright 2021 The Kubernetes Authors.
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

set -o errexit
set -o nounset
set -o pipefail

# darwin is great
SED=sed
if which gsed &>/dev/null; then
  SED=gsed
fi
if ! (${SED} --version 2>&1 | grep -q GNU); then
  echo "!!! GNU sed is required.  If on OS X, use 'brew install gnu-sed'." >&2
  exit 1
fi

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd $REPO_ROOT

echo "Ensuring go version."
source ./hack/build/setup-go.sh

# build codegen tools
echo "Install codegen tools."
cd "hack/tools"
clientgen=${REPO_ROOT}/_bin/client-gen
go build -o "${REPO_ROOT}/_bin/client-gen" k8s.io/code-generator/cmd/client-gen
deepcopygen=${REPO_ROOT}/_bin/deepcopy-gen
go build -o "${REPO_ROOT}/_bin/deepcopy-gen" k8s.io/code-generator/cmd/deepcopy-gen
informergen=${REPO_ROOT}/_bin/informer-gen
go build -o "${REPO_ROOT}/_bin/informer-gen" k8s.io/code-generator/cmd/informer-gen
listergen=${REPO_ROOT}/_bin/lister-gen
go build -o "${REPO_ROOT}/_bin/lister-gen" k8s.io/code-generator/cmd/lister-gen
go_bindata=${REPO_ROOT}/_bin/go-bindata
go build -o "${REPO_ROOT}/_bin/go-bindata" github.com/go-bindata/go-bindata/v3/go-bindata
controller_gen=${REPO_ROOT}/_bin/controller-gen
go build -o "${REPO_ROOT}/_bin/controller-gen" sigs.k8s.io/controller-tools/cmd/controller-gen
echo "Finished installations."
do_clean=${1:-}

cd "${REPO_ROOT}"

# FAKE_GOPATH is for mimicking GOPATH layout.
# K8s code-generator tools all assume the structure of ${GOPATH}/src/k8s.io/...,
# faking GOPATH so that the output are dropped correctly.
# All the clean/copy functions below are for transferring output to this repo.
FAKE_GOPATH=""

cleanup() {
  if [[ -n ${FAKE_GOPATH:-} ]]; then chmod -R u+rwx $FAKE_GOPATH && rm -rf $FAKE_GOPATH; fi
  if [[ -n ${TEMP_GOCACHE:-} ]]; then rm -rf $TEMP_GOCACHE; fi
}
trap cleanup EXIT

ensure-in-gopath() {
  FAKE_GOPATH=$(mktemp -d -t codegen.gopath.XXXX)

  fake_repopath=$FAKE_GOPATH/src/k8s.io/test-infra
  mkdir -p "$(dirname "$fake_repopath")"
  if [[ -n "$do_clean" ]]; then
    cp -LR "${REPO_ROOT}/" "$fake_repopath"
  else
    cp -R "${REPO_ROOT}/" "$fake_repopath"
  fi

  export GOPATH=$FAKE_GOPATH
  cd "$fake_repopath"
}

gen-prow-config-documented() {
  go run ./hack/gen-prow-documented
}

# copyfiles will copy all files in 'path' in the fake gopath over to the
# workspace directory as the code generators output directly into GOPATH,
# meaning without this function the generated files are left in /tmp
copyfiles() {
  path=$1
  name=$2
  if [[ ! -d "$path" ]]; then
    return 0
  fi
  (
    cd "$GOPATH/src/k8s.io/test-infra/$path"
    find "." -name "$name" -exec cp {} "$REPO_ROOT/$path/{}" \;
  )
}

# clean will delete files matching name in path.
#
# When inside bazel test the files are read-only.
# Any attempts to write a file that already exists will fail.
# So resolve by deleting the files before generating them.
clean() {
  path=$1
  name=$2
  if [[ ! -d "$path" || -z "$do_clean" ]]; then
    return 0
  fi
  find "$path" -name "$name" -delete
  find "${REPO_ROOT}"/"$path" -name "$name" -delete
}

gen-deepcopy() {
  clean prow/apis 'zz_generated.deepcopy.go'
  clean prow/config 'zz_generated.deepcopy.non_k8s.go'
  echo "Generating DeepCopy() methods..." >&2
  "$deepcopygen" \
    --go-header-file hack/boilerplate/boilerplate.generated.go.txt \
    --input-dirs k8s.io/test-infra/prow/apis/prowjobs/v1 \
    --output-file-base zz_generated.deepcopy \
    --bounding-dirs k8s.io/test-infra/prow/apis
  copyfiles "prow/apis" "zz_generated.deepcopy.go"

  "$deepcopygen" \
    --go-header-file hack/boilerplate/boilerplate.generated.go.txt \
    --input-dirs k8s.io/test-infra/prow/config \
    --output-file-base zz_generated.deepcopy
  copyfiles "prow/config" "zz_generated.deepcopy.go"

}

gen-client() {
  clean prow/client/clientset '*.go'
  echo "Generating client..." >&2
  "$clientgen" \
    --go-header-file hack/boilerplate/boilerplate.generated.go.txt \
    --clientset-name versioned \
    --input-base "" \
    --input k8s.io/test-infra/prow/apis/prowjobs/v1 \
    --output-package k8s.io/test-infra/prow/client/clientset
  copyfiles "./prow/client/clientset" "*.go"

  clean prow/pipeline/clientset '*.go'
  echo "Generating client for pipeline..." >&2
  "$clientgen" \
    --go-header-file hack/boilerplate/boilerplate.generated.go.txt \
    --clientset-name versioned \
    --input-base "" \
    --input github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1 \
    --output-package k8s.io/test-infra/prow/pipeline/clientset
  copyfiles "./prow/pipeline/clientset" "*.go"
}

gen-lister() {
  clean prow/client/listers '*.go'
  echo "Generating lister..." >&2
  "$listergen" \
    --go-header-file hack/boilerplate/boilerplate.generated.go.txt \
    --input-dirs k8s.io/test-infra/prow/apis/prowjobs/v1 \
    --output-package k8s.io/test-infra/prow/client/listers
  copyfiles "./prow/client/listers" "*.go"

  clean prow/pipeline/listers '*.go'
  echo "Generating lister for pipeline..." >&2
  "$listergen" \
    --go-header-file hack/boilerplate/boilerplate.generated.go.txt \
    --input-dirs github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1 \
    --output-package k8s.io/test-infra/prow/pipeline/listers
  copyfiles "./prow/pipeline/listers" "*.go"
}

gen-informer() {
  clean prow/client/informers '*.go'
  echo "Generating informer..." >&2
  "$informergen" \
    --go-header-file hack/boilerplate/boilerplate.generated.go.txt \
    --input-dirs k8s.io/test-infra/prow/apis/prowjobs/v1 \
    --versioned-clientset-package k8s.io/test-infra/prow/client/clientset/versioned \
    --listers-package k8s.io/test-infra/prow/client/listers \
    --output-package k8s.io/test-infra/prow/client/informers
  copyfiles "./prow/client/informers" "*.go"

  clean prow/pipeline/informers '*.go'
  echo "Generating informer for pipeline..." >&2
  "$informergen" \
    --go-header-file hack/boilerplate/boilerplate.generated.go.txt \
    --input-dirs github.com/tektoncd/pipeline/pkg/apis/pipeline/v1beta1 \
    --versioned-clientset-package k8s.io/test-infra/prow/pipeline/clientset/versioned \
    --listers-package k8s.io/test-infra/prow/pipeline/listers \
    --output-package k8s.io/test-infra/prow/pipeline/informers
  copyfiles "./prow/pipeline/informers" "*.go"
}

gen-spyglass-bindata(){
  cd prow/spyglass/lenses/common/
  echo "Generating spyglass bindata..." >&2
  $go_bindata -pkg=common static/
  gofmt -s -w ./
  cd - >/dev/null
}

gen-prowjob-crd(){
  clean "./config/prow/cluster" "prowjob_customresourcedefinition.yaml"
  echo "Generating prowjob crd..." >&2
  if [[ -z ${HOME:-} ]]; then export HOME=$PWD; fi
  $controller_gen crd:preserveUnknownFields=false,crdVersions=v1 paths=./prow/apis/prowjobs/v1 output:stdout \
    | $SED '/^$/d' \
    | $SED '/^spec:.*/a  \  preserveUnknownFields: false' \
    | $SED '/^  annotations.*/a  \    api-approved.kubernetes.io: https://github.com/kubernetes/test-infra/pull/8669' \
    | $SED '/^          status:/r'<(cat<<EOF
            anyOf:
            - not:
                properties:
                  state:
                    enum:
                    - "success"
                    - "failure"
                    - "error"
            - required:
              - completionTime
EOF
    ) > ./config/prow/cluster/prowjob-crd/prowjob_customresourcedefinition.yaml
  copyfiles "./config/prow/cluster/prowjob-crd" "prowjob_customresourcedefinition.yaml"
  unset HOME
}

gen-prow-config-documented

export GO111MODULE=off
ensure-in-gopath
old=${GOCACHE:-}
export TEMP_GOCACHE=$(mktemp -d -t codegen.gocache.XXXX)
export GOCACHE=$TEMP_GOCACHE
export GO111MODULE=on
export GOPROXY=https://proxy.golang.org
export GOSUMDB=sum.golang.org
go mod vendor
export GO111MODULE=off
export GOCACHE=$old
gen-deepcopy
gen-client
gen-lister
gen-informer
gen-spyglass-bindata
gen-prowjob-crd
export GO111MODULE=on
