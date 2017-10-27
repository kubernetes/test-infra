#!/bin/bash

# Copyright 2017 The Kubernetes Authors.
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

# This script sets up the .netrc file with the supplied token, then pushes to
# the remote repo.
# The script assumes that the working directory is the root of the repo.

set -o errexit
set -o nounset
set -o pipefail

if [ ! $# -eq 2 ]; then
    echo "usage: $0 token branch"
    exit 1
fi

TOKEN="${1}"
BRANCH="${2}"
readonly TOKEN BRANCH

# set up github token in /netrc/.netrc
echo "machine github.com login ${TOKEN}" > /netrc/.netrc
cleanup_github_token() {
    rm -rf /netrc/.netrc
}
trap cleanup_github_token EXIT SIGINT

HOME=/netrc git push origin "${BRANCH}" --no-tags
HOME=/netrc ../push-tags-$(basename "${PWD}")-${BRANCH}.sh
