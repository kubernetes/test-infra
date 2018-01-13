#!/bin/bash
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


# Identifies go_library() bazel targets in the workspace with no dependencies.
# Deletes the target and its files. Cleans up any :all-srcs references

# By default just checks for extra libraries, run with --fix to make changes.

set -o nounset
set -o errexit
set -o pipefail
set -o xtrace

# Remove //foo:something references from bar/BUILD
# Remove //vendor/golang.org/x/text/internal:go_default_library deps
#
# Unused text/gen.go file imports text/internal, which prevents dep prune from
# removing it. However text/internal package references a text/language package
# which has been pruned.
#
# Fix by deleting this dependency. The target won't build correctly, but it
# won't anyway because text/language doesn't exist, and it will get pruned by
# hack/prune-libraries.sh
#
# Usage:
#   remove-text-internal
drop-dep() {
  local path
  path="$(dirname "${BASH_SOURCE}")/../$2/BUILD"
  if [[ ! -f ${path} ]]; then
    return 0
  fi
  sed -i -e "\|//$1:go_default_library|d" "$path"
}

trap 'echo "FAILED" >&2' ERR
pushd "$(dirname "${BASH_SOURCE}")/.."
dep ensure -v
dep prune -v
hack/update-bazel.sh
drop-dep vendor/golang.org/x/text/language vendor/golang.org/x/text/internal
drop-dep vendor/google.golang.org/api/transport/grpc vendor/google.golang.org/api/transport
drop-dep vendor/github.com/golang/protobuf/protoc-gen-go/grpc vendor/github.com/golang/protobuf/protoc-gen-go
drop-dep vendor/github.com/golang/protobuf/protoc-gen-go/generator vendor/github.com/golang/protobuf/protoc-gen-go
drop-dep vendor/github.com/docker/docker/pkg/ioutils vendor/github.com/docker/docker/api
drop-dep vendor/github.com/docker/docker/pkg/system vendor/github.com/docker/docker/api
drop-dep vendor/github.com/docker/libtrust vendor/github.com/docker/docker/api
drop-dep vendor/github.com/docker/distribution/context vendor/github.com/docker/distribution
hack/prune-libraries.sh --fix
hack/update-bazel.sh  # Update child :all-srcs in case parent was deleted
echo SUCCESS
