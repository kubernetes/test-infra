#!/bin/bash
# Copyright 2016 The Kubernetes Authors.
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

if [ "$#" -lt 1 ]; then
  echo "usage: $0 <program> [program ...]"
  exit 1
fi

# darwin is great
SED=sed
if which gsed &>/dev/null; then
  SED=gsed
fi
if ! ($SED --version 2>&1 | grep -q GNU); then
  echo "!!! GNU sed is required.  If on OS X, use 'brew install gnu-sed'."
  exit 1
fi

cd $(dirname $0)

new_version="v$(date -u '+%Y%m%d')-$(git describe --tags --always --dirty)"
for i in "$@"; do
  echo "program: $i"
  echo "new version: $new_version"
  gcloud docker -- pull "${PREFIX:-gcr.io/k8s-prow}/${i}:${new_version}"

  $SED -i "s/\(${i}:\)v[a-f0-9-]\+/\1$new_version/I" cluster/*.yaml
done
