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

# This file includes functions shared by the each repository's publish scripts.

# sync() cherry picks the latest changes in k8s.io/kubernetes/${filter} to the
# local copy of the repository to be published.
#
# overall flow
# 1. fetch the current level of k8s.io/kubernetes
# 2. check out the k8s.io/kubernetes HEAD into a separate branch
# 3. rewrite the history on that branch to *only* include staging/src/k8s.io/apimachinery
# 4. locate all commits between the last time we sync'ed and now
# 5. switch back to the starting branch
# 6. for each commit, cherry-pick it (which will keep authorship) into current branch
# 7. update metadata files indicating which commits we've sync'ed to
#
# The function assumes to be called at the root of the repository that's going to be published.
sync_repo() {
    # filter could be staging/src/k8s.io/apimachinery
    filter="${1}"
    src_branch="${2}"
    readonly filter src_branch
    set -e
    dir=$(mktemp -d "${TMPDIR:-/tmp/}$(basename $0).XXXXXXXXXXXX")

    currBranch=$(git rev-parse --abbrev-ref HEAD)
    previousKubeSHA=$(cat kubernetes-sha)
    previousBranchSHA=$(cat filter-branch-sha)
    
    git remote add upstream-kube https://github.com/kubernetes/kubernetes.git || true
    git fetch upstream-kube
    git branch -D kube-sync || true
    git checkout upstream-kube/"${src_branch}" -b kube-sync
    git reset --hard upstream-kube/"${src_branch}"
    newKubeSHA=$(git log --oneline --format='%H' kube-sync -1)
    
    # this command rewrite git history to *only* include $filter 
    git filter-branch -f --subdirectory-filter "${filter}" HEAD
    
    newBranchSHA=$(git log --oneline --format='%H' kube-sync -1)
    #git log --no-merges --format='%H' --reverse ${previousBranchSHA}..HEAD^ > ${dir}/commits
    # we need to start after _vendor is removed from master
    git log --no-merges --format='%H' --reverse ${previousBranchSHA}..HEAD > ${dir}/commits
    
    git checkout ${currBranch}

    echo "commits to be published:"
    cat ${dir}/commits
    echo ""
    
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

# To avoid repeated godep restore, repositories should share the GOPATH.
# This function should be run after the Godeps.json are updated with the latest
# revs of k8s.io/ dependencies.
# The function assumes to be called at the root of the repository that's going to be published.
restore_vendor() {
    godep restore
    godep save ./...
    rm -rf ./vendor/k8s.io

    # glog uses global variables, it panics when multiple copies are compiled.
    rm -rf ./vendor/github.com/golang/glog
    # this ensures users who get the repository via `go get` won't end up with
    # multiple copies of k8s.io/ repos. The only copy will be the one in the
    # GOPATH.
    # Godeps.json has a complete, up-to-date list of dependencies, so
    # Godeps.json will be the ground truth for users using godep/glide/dep.
    rm -rf ./vendor/k8s.io  
}
