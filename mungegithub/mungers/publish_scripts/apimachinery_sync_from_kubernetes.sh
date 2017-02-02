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


# overall flow
# 1. fetch the current level of k8s.io/kubernetes
# 2. check out the k8s.io/kubernetes HEAD into a separate branch
# 3. rewrite the history on that branch to *only* include staging/src/k8s.io/apimachinery
# 4. locate all commits between the last time we sync'ed and now
# 5. switch back to the starting branch
# 6. for each commit, cherry-pick it (which will keep authorship) into current branch
# 7. update metadata files indicating which commits we've sync'ed to

set -e

if [ ! $# -eq 2 ]; then
    echo "usage: apimachinery-sync-from-kubernetes.sh token netrc_dir. netrc_dir is expected to be absolute paths."
    exit 1
fi

TOKEN="${1}"
NETRCDIR="${2}"

# set up github token
echo "machine github.com login ${TOKEN}" > "${NETRCDIR}"/.netrc
rm -f ~/.netrc
ln -s "${NETRCDIR}"/.netrc ~/.netrc
# set up github user
git config --global user.email "k8s-publish-robot@users.noreply.github.com"
git config --global user.name "Kubernetes Publisher"

dir=$(mktemp -d "${TMPDIR:-/tmp/}$(basename 0).XXXXXXXXXXXX")

git remote add upstream-kube https://github.com/kubernetes/kubernetes.git || true
git fetch upstream-kube

currBranch=$(git rev-parse --abbrev-ref HEAD)
previousKubeSHA=$(cat kubernetes-sha)
previousBranchSHA=$(cat filter-branch-sha)

# hack...
if [ "$previousBranchSHA" == acda1e7401fbf79d89ba23df5e11d81f4a9d4e0d ]; then
    previousBranchSHA=f6eb43d54a86a6521ef4bcebc22e4bb249e3213f
fi

if [ "$previousBranchSHA" == 442d134903e380b749aadecebe4a9e26b1faa6b6 ]; then
    previousBranchSHA=30f11579e544d22b567f9162ecde708384160372
fi

git branch -D kube-sync || true
git checkout upstream-kube/master -b kube-sync
git reset --hard upstream-kube/master
newKubeSHA=$(git log --oneline --format='%H' kube-sync -1)

# this command rewrite git history to *only* include staging/src/k8s.io/apimachinery
git filter-branch -f --subdirectory-filter staging/src/k8s.io/apimachinery HEAD

newBranchSHA=$(git log --oneline --format='%H' kube-sync -1)
git log --no-merges --format='%H' --reverse ${previousBranchSHA}..HEAD > ${dir}/commits

git checkout ${currBranch}

while read commitSHA; do
	echo "working ${commitSHA}"
	git cherry-pick ${commitSHA}
done <${dir}/commits

# track the k8s.io/kubernetes commit SHA so we can always determine which level of kube this repo matches
# track the filtered branch commit SHA so that we can determine which commits need to be picked
echo ${newKubeSHA} > kubernetes-sha
echo ${newBranchSHA} > filter-branch-sha
git commit -m "sync(k8s.io/kubernetes): ${newKubeSHA}" -- kubernetes-sha filter-branch-sha

git push origin master
rm -f ~/.netrc
rm -f "${NETRCDIR}"/.netrc

