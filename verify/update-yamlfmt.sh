#!/usr/bin/env bash

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

ROOT_DIR=${ROOT_DIR:-$(dirname "${BASH_SOURCE}")/..}

if ! which yamlfmt >/dev/null; then
  echo "!!! yamlfmt must be installed:" >&2
  echo "go get -u github.com/frankbraun/kitchensink/cmd/yamlfmt" >&2
  exit 1
fi

# We need to create a tmpdir for yamlfmt on the same device as the repo;
# otherwise its os.Rename call may fail.
export TMPDIR=$(mktemp -d "${ROOT_DIR}/_tmp-update-yamlfmt.XXX")
trap "rm -r ${TMPDIR}" EXIT
find "${ROOT_DIR}" -iname '*.yaml' | xargs -n 1 yamlfmt
