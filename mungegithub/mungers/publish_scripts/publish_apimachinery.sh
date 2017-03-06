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

# This script publishes the latest changes in the master branch of
# k8s.io/kubernetes/staging/src/apimachinery to the master branch of
# k8s.io/apimachinery.
#
# The script assumes that the working directory is
# $GOPATH/src/k8s.io/apimachinery/, the master branch is checked out and is
# tracking the master branch of https://github.com/kubernetes/apimachinery.
#
# The script is expected to be run by
# k8s.io/test-infra/mungegithub/mungers/publisher.go

if [ ! $# -eq 2 ]; then
    echo "usage: $0 token"
    exit 1
fi

TOKEN="${1}"

# set up github token
mv ~/.netrc ~/.netrc.bak || true
echo "machine github.com login ${TOKEN}" > ~/.netrc

cleanup() {
    rm -rf ~/.netrc
    mv ~/.netrc.bak ~/.netrc || true
}
trap cleanup EXIT SIGINT

sync

git push origin $(git rev-parse --abbrev-ref HEAD)

# sync() cherry picks the latest changes in
# k8s.io/kubernetes/staging/src/apimachinery to the local copy of
# the k8s.io/apimachinery repository.
#
# overall flow
# 1. fetch the current level of k8s.io/kubernetes
# 2. check out the k8s.io/kubernetes HEAD into a separate branch
# 3. rewrite the history on that branch to *only* include staging/src/k8s.io/apimachinery
# 4. locate all commits between the last time we sync'ed and now
# 5. switch back to the starting branch
# 6. for each commit, cherry-pick it (which will keep authorship) into current branch
# 7. update metadata files indicating which commits we've sync'ed to

sync() {
    set -e
    dir=$(mktemp -d "${TMPDIR:-/tmp/}$(basename $0).XXXXXXXXXXXX")
    
    git remote add upstream-kube https://github.com/kubernetes/kubernetes.git || true
    git fetch upstream-kube
    
    currBranch=$(git rev-parse --abbrev-ref HEAD)
    previousKubeSHA=$(cat kubernetes-sha)
    previousBranchSHA=$(cat filter-branch-sha)
    
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
    git -c user.name="Kubernetes Publisher" -c user.email="k8s-publish-robot@users.noreply.github.com" commit -m "sync(k8s.io/kubernetes): ${newKubeSHA}" -- kubernetes-sha filter-branch-sha
}
