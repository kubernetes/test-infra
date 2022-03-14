#!/usr/bin/env bash
# Copyright 2022 The Kubernetes Authors.
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

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)"
cd $REPO_ROOT

GRPC="plugins=grpc"
IMPORT_MAP="Mtestgrid/config/config.proto=k8s.io/test-infra/testgrid/config,Mtestgrid/summary/summary.proto=k8s.io/test-infra/testgrid/summary,Mtestgrid/cmd/summarizer/response/types.proto=k8s.io/test-infra/testgrid/cmd/summarizer/response"
BOILERPLATE_FILE="hack/boilerplate/boilerplate.generated.go.txt"

echo "Install proto generating tools."
# Install protoc
if [[ ! -f "_bin/protoc/bin/protoc" ]]; then
  mkdir -p _bin/protoc
  PROTOC_ZIP=protoc-3.15.5-linux-x86_64.zip
  if [[ "$(uname)" == Darwin ]]; then
    PROTOC_ZIP=protoc-3.15.5-osx-x86_64.zip
  fi
  curl -OL https://github.com/protocolbuffers/protobuf/releases/download/v3.15.5/$PROTOC_ZIP
  unzip -o $PROTOC_ZIP -d _bin/protoc bin/protoc
  unzip -o $PROTOC_ZIP -d _bin/protoc 'include/*'
  rm -f $PROTOC_ZIP
fi

echo "Ensuring go version."
source ./hack/build/setup-go.sh

cd "hack/tools"
PROTOC_GEN_GO="${REPO_ROOT}/_bin/protoc-gen-go"
go build -o "${PROTOC_GEN_GO}" github.com/golang/protobuf/protoc-gen-go
cd "${REPO_ROOT}"

genproto() {
  dir=$(dirname "$1")
  base=$(basename "$1")
  out=${REPO_ROOT}/$dir/${base%.proto}.pb.go
  rm -f "$out" # mac will complain otherwise
  export PATH="${REPO_ROOT}/_bin/protoc/include/google/protobuf;${REPO_ROOT}/_bin;$PATH"
  ./_bin/protoc/bin/protoc "--plugin=${PROTOC_GEN_GO}" "--proto_path=${dir}" "--proto_path=${REPO_ROOT}" "--go_out=${GRPC},${IMPORT_MAP}:$REPO_ROOT/$dir" "$1"
  tmp=$(mktemp)
  mv "$out" "$tmp"
  cat "$boiler" "$tmp" > "$out"
}

echo -n "Generating protos: " >&2
for p in $(find . \
  -not '(' -path './vendor' -prune ')' \
  -not '(' -path './node_modules' -prune ')' \
  -not '(' -path './_bin' -prune ')' \
  -name '*.proto'); do
  echo "Generating for: $p"
  echo -n "$p "
  genproto "$p"
done
echo