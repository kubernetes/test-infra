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

set -o errexit
set -o nounset
set -o pipefail

if [ ! $# -eq 1 ]; then
    echo "usage: $0 github_user_name"
    exit 1
fi

USER="${1}"
repos=(
    apimachinery
    api
    client-go
    apiserver
    kube-aggregator
    sample-apiserver
    apiextensions-apiserver
    metrics
    code-generator
)

repo_count=${#repos[@]}

TMPDIR=$(mktemp -d)
function delete() {
    echo "Deleting ${TMPDIR}..."
    rm -rf "${TMPDIR}"
}
trap delete EXIT INT

# safety check
if [ "$USER" = "kubernetes" ]; then
    echo "Cannot operate on kubernetes directly" 1>&2
    exit 1
fi

echo "==================="
echo " sync with upstream"
echo "==================="
for (( i=0; i<${repo_count}; i++ )); do
    git clone git@github.com:"${USER}/${repos[i]}".git ${TMPDIR}/"${repos[i]}"
    pushd ${TMPDIR}/"${repos[i]}"
    git remote add upstream git@github.com:kubernetes/"${repos[i]}".git

    # delete all branches in origin
    branches=$(git branch -r | grep "^ *origin" | sed 's,^ *origin/,,' | grep -v HEAD | grep -v '^master' || true)
    if [ -n "${branches}" ]; then
        git push --delete origin ${branches}
    fi

    # push all upstream branches to origin
    git fetch upstream --prune
    branches=$(git branch -r | grep "^ *upstream" | sed 's,^ *upstream/,,' | grep -v HEAD || true)
    for branch in ${branches}; do
        git push --no-tags -f origin upstream/${branch}:refs/heads/${branch}
    done

    popd
done
