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

# BSD sed doesn't have --version, and needs + instead of /
# GNU sed deprecated and removed +
desired_perm="/111"
# we're not actually running a search so SC2185 doesn't apply
# shellcheck disable=SC2185
if ! find --version >/dev/null 2>&1; then
  desired_perm="+111"
fi

# find all files named *.sh (approximate shell script detection ...)
# - ignoring .git
# - that are not executable by all
files=$(find . -type f -name '*.sh' -not -perm "${desired_perm}" -not -path './.git/*')
if [[ -n "${files}" ]]; then
  echo "${files}"
  echo
  echo "Please run hack/update-file-perms.sh"
  exit 1
fi
