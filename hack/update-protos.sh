#!/usr/bin/env bash
# Copyright 2019 The Kubernetes Authors.
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
  echo "Updating protos..." >&2
elif ! command -v bazel &>/dev/null; then
  echo "Install bazel at https://bazel.build" >&2
  exit 1
else
  (
    set -o xtrace
    bazel run //hack:update-protos
  )
  exit 0
fi

protoc=$1
plugin=$2
boiler=$3
grpc=$4
importmap=$5
dest=$BUILD_WORKSPACE_DIRECTORY

genproto() {
  dir=$(dirname "$1")
  base=$(basename "$1")
  out=$dest/$dir/${base%.proto}.pb.go
  rm -f "$out" # mac will complain otherwise
  "$protoc" "--plugin=$plugin" "--proto_path=${dir}" "--proto_path=${dest}" "--go_out=${grpc},${importmap}:$dest/$dir" "$1"
  tmp=$(mktemp)
  mv "$out" "$tmp"
  cat "$boiler" "$tmp" > "$out"
}

echo -n "Generating protos: " >&2
for p in $(find . -not '(' -path './vendor' -prune ')' -not '(' -path './node_modules' -prune ')' -name '*.proto'); do
  echo -n "$p "
  genproto "$p"
done
echo

