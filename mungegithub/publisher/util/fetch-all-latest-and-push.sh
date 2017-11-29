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

if [ "$#" = 0 ] || [ "$#" -gt 2 ]; then
    echo "usage: $0 [source-github-user-name] dest-github-user-name"
    echo
    echo "This connects to git@github.com:<from>/<repo>. Set GITHUB_HOST to access git@<GITHUB_HOST>:<from>/<repo> instead."
    exit 1
fi

FROM="kubernetes"
TO="${1}"
if [ "$#" -ge 2 ]; then
    FROM="${TO}"
    TO="${2}"
fi
GITHUB_HOST=${GITHUB_HOST:-github.com}
repos=(
    apimachinery
    api
    client-go
    apiserver
    kube-aggregator
    sample-apiserver
    sample-controller
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
if [ "${TO}" = "kubernetes" ]; then
    echo "Cannot operate on kubernetes directly" 1>&2
    exit 1
fi

echo "==================="
echo " sync with upstream"
echo "==================="
for (( i=0; i<${repo_count}; i++ )); do
    git clone git@${GITHUB_HOST}:"${TO}/${repos[i]}".git ${TMPDIR}/"${repos[i]}"
    pushd ${TMPDIR}/"${repos[i]}"
    git remote add upstream git@${GITHUB_HOST}:"${FROM}/${repos[i]}".git

    # delete all tags and branches in origin
    rm -f .git/refs/tags/*
    branches=$(git branch -r | grep "^ *origin" | sed 's,^ *origin/,,' | grep -v HEAD | grep -v '^master' || true)
    tags=$(git tag | sed 's,^,refs/tags/,')
    if [ -n "${branches}${tags}" ]; then
        git push --delete origin ${branches} ${tags}
    fi

    # push all upstream tags and branches to origin
    git tag | xargs git tag -d
    git fetch upstream --prune -q
    branches=$(git branch -r | grep "^ *upstream" | sed 's,^ *upstream/,,' | grep -v HEAD || true)
    branches_arg=""
    for branch in ${branches}; do
        branches_arg+=" upstream/${branch}:refs/heads/${branch}"
    done
    git push --tags -f origin ${branches_arg}

    popd
done
