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

# sync_repo() cherry picks the latest changes in k8s.io/kubernetes/${filter} to the
# local copy of the repository to be published.
#
# prerequisites
# 1. we are in the root of the repository to be published
# 2. we are on the branch to be published (let's call it "target-branch")
# overall flow
# 1. fetch the current level of k8s.io/kubernetes
# 2. check out the $src_branch of k8s.io/kubernetes as branch kube-sync
# 3. rewrite the history of branch kube-sync to *only* include code in $subdirectory
# 4. locate all commits between the last time we sync'ed and now
# 5. switch back to the "target-branch"
# 6. for each commit, cherry-pick it (which will keep authorship) into "target-branch"
# 7. update metadata files indicating which commits we've sync'ed to
sync_repo() {
    # subdirectory in k8s.io/kubernetes, e.g., staging/src/k8s.io/apimachinery
    local subdirectory="${1}"
    local src_branch="${2}"
    readonly filter src_branch

    local currBranch=$(git rev-parse --abbrev-ref HEAD)
    local previousKubeSHA=$(cat kubernetes-sha)
    
    git remote add upstream-kube https://github.com/kubernetes/kubernetes.git || true
    git fetch upstream-kube
    git branch -D kube-sync || true
    git checkout upstream-kube/"${src_branch}" -b kube-sync
    git reset --hard upstream-kube/"${src_branch}"
    
    # this command rewrites git history to *only* include $subdirectory 
    git filter-branch -f --msg-filter 'awk 1 && echo && echo "Kubernetes-commit: ${GIT_COMMIT}"' \
        --subdirectory-filter "${subdirectory}" HEAD

    local newKubeSHA=$(git log kube-sync -1 | tail -n 1 | sed "s/Kubernetes-commit: //g")

    local previousBranchSHA=$(git log --grep "Kubernetes-commit: ${previousKubeSHA}" --format='%H')
    local commits=$(git log --no-merges --format='%H' --reverse ${previousBranchSHA}..HEAD)

    git checkout ${currBranch}

    echo "commits to be cherry-picked:"
    echo "${commits}"
    echo ""
    
    while read commitSHA; do
        if [[ -z "${commitSHA}" ]]; then
            continue
        fi
    	echo "working ${commitSHA}"
    	git cherry-pick ${commitSHA}
    done <<< "${commits}"
    
    # track the k8s.io/kubernetes commit SHA so we can always determine which level of kube this repo matches
    # track the filtered branch commit SHA so that we can determine which commits need to be picked
    echo ${newKubeSHA} > kubernetes-sha
    if git diff --exit-code &>/dev/null; then
        echo "SHAs haven't changed!"
        return
    fi
    git -c user.name="Kubernetes Publisher" -c user.email="k8s-publish-robot@users.noreply.github.com" commit -m "sync(k8s.io/kubernetes): ${newKubeSHA}" -- kubernetes-sha
}

# To avoid repeated godep restore, repositories should share the GOPATH.
# This function should be run after the Godeps.json are updated with the latest
# revs of k8s.io/ dependencies.
# The function assumes to be called at the root of the repository that's going to be published.
restore_vendor() {
    godep restore
    # need to remove the Godeps folder, otherwise godep won't save source code to vendor/
    mv ./Godeps ./Godeps.backup
    godep save ./...
    rm -rf ./Godeps
    mv ./Godeps.backup ./Godeps
    # glog uses global variables, it panics when multiple copies are compiled.
    rm -rf ./vendor/github.com/golang/glog
    # this ensures users who get the repository via `go get` won't end up with
    # multiple copies of k8s.io/ repos. The only copy will be the one in the
    # GOPATH.
    # Godeps.json has a complete, up-to-date list of dependencies, so
    # Godeps.json will be the ground truth for users using godep/glide/dep.
    rm -rf ./vendor/k8s.io  
    git add --all
    # check if there are new contents 
    if git diff --cached --exit-code &>/dev/null; then
        echo "vendor hasn't changed!"
        return
    fi
    git -c user.name="Kubernetes Publisher" -c user.email="k8s-publish-robot@users.noreply.github.com" commit -m "Update vendor/"
}

# set up github token in ~/.netrc
set_github_token() {
    mv ~/.netrc ~/.netrc.bak || true
    echo "machine github.com login ${1}" > ~/.netrc
}

cleanup_github_token() {
    rm -rf ~/.netrc
    mv ~/.netrc.bak ~/.netrc || true
}
