#!/usr/bin/env bash
# Copyright 2018 The Kubernetes Authors.
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

if [[ -n "${BUILD_WORKSPACE_DIRECTORY:-}" ]]; then # Running inside bazel
  echo "Updating codegen files..." >&2
elif ! command -v bazel &>/dev/null; then
  echo "Install bazel at https://bazel.build" >&2
  exit 1
else
  (
    set -o xtrace
    bazel run @io_k8s_test_infra//hack:update-codegen
  )
  exit 0
fi

go_sdk=$PWD/external/go_sdk
clientgen=$PWD/$1
deepcopygen=$PWD/$2
informergen=$PWD/$3
listergen=$PWD/$4
go_bindata=$PWD/$5
do_clean=${6:-}

ensure-in-gopath() {
  fake_gopath=$(mktemp -d -t codegen.gopath.XXXX)

  fake_repopath=$fake_gopath/src/k8s.io/test-infra
  mkdir -p "$(dirname "$fake_repopath")"
  if [[ -n "$do_clean" ]]; then
    cp -LR "$BUILD_WORKSPACE_DIRECTORY/" "$fake_repopath"
  else
    cp -R "$BUILD_WORKSPACE_DIRECTORY/" "$fake_repopath"
  fi

  export GOPATH=$fake_gopath
  export GOROOT=$go_sdk
  cd "$fake_repopath"
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
    find "." -name "$name" -exec cp {} "$BUILD_WORKSPACE_DIRECTORY/$path/{}" \;
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
  find "$BUILD_WORKSPACE_DIRECTORY"/"$path" -name "$name" -delete
}

gen-deepcopy() {
  clean prow/apis 'zz_generated.deepcopy.go'
  echo "Generating DeepCopy() methods..." >&2
  "$deepcopygen" \
    --go-header-file hack/boilerplate/boilerplate.generated.go.txt \
    --input-dirs k8s.io/test-infra/prow/apis/prowjobs/v1 \
    --output-file-base zz_generated.deepcopy \
    --bounding-dirs k8s.io/test-infra/prow/apis
  copyfiles "prow/apis" "zz_generated.deepcopy.go"
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
    --input github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1 \
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
    --input-dirs github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1 \
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
    --input-dirs github.com/tektoncd/pipeline/pkg/apis/pipeline/v1alpha1 \
    --versioned-clientset-package k8s.io/test-infra/prow/pipeline/clientset/versioned \
    --listers-package k8s.io/test-infra/prow/pipeline/listers \
    --output-package k8s.io/test-infra/prow/pipeline/informers
  copyfiles "./prow/pipeline/informers" "*.go"
}

gen-spyglass-bindata(){
  cd prow/spyglass/lenses/common/
  $go_bindata -pkg=common static/
  "$go_sdk/bin/gofmt" -s -w ./
  cd -
}

export GO111MODULE=off
ensure-in-gopath
old=${GOCACHE:-}
export GOCACHE=$(mktemp -d -t codegen.gocache.XXXX)
export GO111MODULE=on
export GOPROXY=https://proxy.golang.org
export GOSUMDB=sum.golang.org
"$go_sdk/bin/go" mod vendor
export GO111MODULE=off
export GOCACHE=$old
gen-deepcopy
gen-client
gen-lister
gen-informer
gen-spyglass-bindata
export GO111MODULE=on
