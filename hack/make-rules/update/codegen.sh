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
go build -o "${clientgen}" k8s.io/code-generator/cmd/client-gen
deepcopygen=${REPO_ROOT}/_bin/deepcopy-gen
go build -o "${deepcopygen}" k8s.io/code-generator/cmd/deepcopy-gen
informergen=${REPO_ROOT}/_bin/informer-gen
go build -o "${informergen}" k8s.io/code-generator/cmd/informer-gen
listergen=${REPO_ROOT}/_bin/lister-gen
go build -o "${listergen}" k8s.io/code-generator/cmd/lister-gen
go_bindata=${REPO_ROOT}/_bin/go-bindata
go build -o "${go_bindata}" github.com/go-bindata/go-bindata/v3/go-bindata
controller_gen=${REPO_ROOT}/_bin/controller-gen
go build -o "${controller_gen}" sigs.k8s.io/controller-tools/cmd/controller-gen
protoc_gen_go="${REPO_ROOT}/_bin/protoc-gen-go" # golang protobuf plugin
GOBIN="${REPO_ROOT}/_bin" go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.1
GOBIN="${REPO_ROOT}/_bin" go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2

cd "${REPO_ROOT}"
ensure-protoc-deps(){
  # Install protoc
  if [[ ! -f "_bin/protoc/bin/protoc" ]]; then
    mkdir -p _bin/protoc
    # See https://developers.google.com/protocol-buffers/docs/news/2022-05-06 for
    # a note on the versioning scheme change.
    PROTOC_VERSION=21.9
    PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-x86_64.zip"
    curl -OL "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}"
    unzip -o $PROTOC_ZIP -d _bin/protoc bin/protoc
    unzip -o $PROTOC_ZIP -d _bin/protoc 'include/*'
    rm -f $PROTOC_ZIP
  fi

  # Clone proto dependencies.
  if ! [[ -f "${REPO_ROOT}"/_bin/protoc/include/googleapis/google/api/annotations.proto ]]; then
    # This SHA was retrieved on 2022-12-14.
    GOOGLEAPIS_VERSION="d9dc42bf24866ac28c09489feb58590c838ed970"
    >/dev/null pushd "${REPO_ROOT}"/_bin/protoc/include
    curl -OL "https://github.com/googleapis/googleapis/archive/${GOOGLEAPIS_VERSION}.zip"
    >/dev/null unzip -o ${GOOGLEAPIS_VERSION}.zip
    mv googleapis-${GOOGLEAPIS_VERSION} googleapis
    rm -f ${GOOGLEAPIS_VERSION}.zip
    >/dev/null popd
  fi
}
ensure-protoc-deps

echo "Finished installations."
do_clean=${1:-}

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

# Generate gRPC stubs for a given protobuf file.
gen-proto-stubs(){
  local dir
  dir="$(dirname "$1")"

  # We need the "paths=source_relative" bits to prevent a nested directory
  # structure (so that the generated files can sit next to the .proto files,
  # instead of under a "k8.io/test-infra/prow/..." subfolder).
  "${REPO_ROOT}/_bin/protoc/bin/protoc" \
    "--plugin=${protoc_gen_go}" \
    "--proto_path=${REPO_ROOT}/_bin/protoc/include/google/protobuf" \
    "--proto_path=${REPO_ROOT}/_bin/protoc/include/googleapis" \
    "--proto_path=${dir}" \
    --go_out="${dir}" --go_opt=paths=source_relative \
    --go-grpc_out="${dir}" --go-grpc_opt=paths=source_relative \
    "$1"
}

gen-all-proto-stubs(){
  echo >&2 "Generating proto stubs"

  # Expose the golang protobuf plugin binaries (protoc-gen-go,
  # protoc-gen-go-grpc) to the PATH so that protoc can find it.
  export PATH="${REPO_ROOT}/_bin:$PATH"

  while IFS= read -r -d '' proto; do
    echo >&2 "  $proto"
    gen-proto-stubs "$proto"
  done < <(find "${REPO_ROOT}" \
    -not '(' -path "${REPO_ROOT}/vendor" -prune ')' \
    -not '(' -path "${REPO_ROOT}/node_modules" -prune ')' \
    -not '(' -path "${REPO_ROOT}/_bin" -prune ')' \
    -name '*.proto' \
    -print0 | sort -z)
}

gen-gangway-apidescriptorpb-for-cloud-endpoints(){
  echo >&2 "Generating self-describing proto stub (api_descriptor.pb) for gangway.proto"

  "${REPO_ROOT}/_bin/protoc/bin/protoc" \
    "--proto_path=${REPO_ROOT}/_bin/protoc/include/google/protobuf" \
    "--proto_path=${REPO_ROOT}/_bin/protoc/include/googleapis" \
    "--proto_path=${REPO_ROOT}/prow/gangway" \
    --include_imports \
    --include_source_info \
    --descriptor_set_out "${REPO_ROOT}/prow/gangway/api_descriptor.pb" \
    gangway.proto
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

gen-all-proto-stubs
gen-gangway-apidescriptorpb-for-cloud-endpoints
