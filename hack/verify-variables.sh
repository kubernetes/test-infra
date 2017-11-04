#!/bin/bash

# Copyright 2014 The Kubernetes Authors.
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

# GoFmt apparently is changing @ head...

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(dirname "${BASH_SOURCE}")/..

function print_file() {
  grep "export" "${1}" | cut -f2 -d" " | cut -f1 -d"=" | sort
}

function find_files() {
  echo $(ls "${ROOT}/jobs/" | grep ".sh")
}

function find_duplicates() {
  if [[ "$(print_file $1 | wc -l)" -ne "$(print_file $1 | uniq | wc -l)" ]]; then
    diff <(print_file $1) <(print_file $1 | uniq) | sed "s/^/^$f /g" | grep "<" 1>&2
    echo "${1}"
  fi
}

bad_files=""

for f in $(find_files); do
  duplicates=$(find_duplicates "${ROOT}/jobs/$f")
  if [[ -n "${duplicates}" ]]; then
    bad_files=$(echo "${bad_files}" && echo "${f}")
  fi
done

if [[ -n "${bad_files}" ]]; then
  echo "Found duplicate declarations in files"
  exit 1
fi
