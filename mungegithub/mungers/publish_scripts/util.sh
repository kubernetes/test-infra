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

set -o errexit
set -o nounset
set -o pipefail

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
    set -x
    # subdirectory in k8s.io/kubernetes, e.g., staging/src/k8s.io/apimachinery
    local subdirectory="${1}"
    local src_branch="${2}"
    local kubernetes_remote="${3:-https://github.com/kubernetes/kubernetes.git}"
    readonly filter src_branch
    local fakeUser='-c user.name="Kubernetes Publisher" -c user.email="k8s-publish-robot@users.noreply.github.com"'
    
    local oldHEAD=$(git rev-parse HEAD)
    local previousKubeSHA=$(cat kubernetes-sha) # || $(git show -q HEAD | tail -n 1 | sed "s/.*Kubernetes-commit: //g"))
    local new_repo="false"
    if [[ "${previousKubeSHA}" == "" ]]; then
        new_repo="true"
        echo "sync_repo() is processing a repo that doesn't have kubernetes-sha, treat it as a new repo"
    fi

    # we remove vendor/ first, but rebase onto parent later to keep the vendor dir/
    #git rm -q -rf vendor/
    #git -c user.name="Kubernetes Publisher" -c user.email="k8s-publish-robot@users.noreply.github.com" commit -m "TEMPORARY remove vendor/" -q --allow-empty
    #local removeVendorHEAD=$(git rev-parse HEAD)
    
    # fetch upstream kube
    git remote add upstream-kube "${kubernetes_remote}" || true
    git fetch upstream-kube
    git branch -f kube-sync upstream-kube/"${src_branch}"

    # we stored kube sha1s from single commits, not from merge commits on the first-parent path to HEAD.
    # Hence, we find the merge-commit of the stored sha and use that as the base for new commits.
    previousKubeMergeSHA=$(git-find-merge ${previousKubeSHA} kube-sync)

    # checkout upstream kube master
    git checkout kube-sync
    git reset --hard
    git clean -f -f -f

    # any changes at all?
    if [ "$(git rev-parse HEAD)" = "${previousKubeSHA}" ]; then
	return 0
    fi
    
    # rewrite git history to
    # - to *only* include $subdirectory
    # - to add a "Kubernetes-commit: <kube-sha>" line to commit message
    # - to add a kubernetes-sha file to the root directory.
    git filter-branch -f --msg-filter 'awk 1 && echo && echo "Kubernetes-commit: ${GIT_COMMIT}"' \
	--index-filter 'echo ${GIT_COMMIT} > kubernetes-sha; git add kubernetes-sha' \
        --subdirectory-filter "${subdirectory}" HEAD
    
    # rewrite parents to point to our existing commits. This will be essential whenever we change
    # anything in the rewrite rules in order to find out old commit and to re-use them, i.e. to
    # build on-top of them. We include the "TEMPORARY remove vendor/" commit to avoid that the first
    # new commit shows a huge vendor/ deletion diff.
    echo > /tmp/foo
    git filter-branch -f --parent-filter '
    	OUT=""
    	for PARENT in $(sed "s/-p //g"); do
	    KUBE_COMMIT=$(git show -q ${PARENT} | tail -n 1 | sed "s/.*Kubernetes-commit: //g")
	    echo "KUBE_COMMIT(${PARENT}) = ${KUBE_COMMIT}" >> /tmp/foo
	    if [ ${KUBE_COMMIT} = '${previousKubeSHA}' ] || [ ${KUBE_COMMIT} = '${previousKubeMergeSHA}' ]; then
	       echo "====================================================" >> /tmp/foo
	       OUT="${OUT} -p '${oldHEAD}'"
            else
               EXISTING_COMMIT=$(git log --grep "Kubernetes-commit: $(echo ${KUBE_COMMIT})" --format="%H" '${oldHEAD}')
	       echo "EXISTING_COMMIT(${KUBE_COMMIT}) = ${EXISTING_COMMIT}" >> /tmp/foo
               OUT="${OUT} -p ${EXISTING_COMMIT:-${PARENT}}"
            fi
	 done
	 echo "OUT: ${OUT}" >> /tmp/foo
	 echo ${OUT}
    ' HEAD
    cat /tmp/foo

    git log --oneline ${oldHEAD}..HEAD

    # rebase new commits onto the head before the "TEMPORARY remove vendor/" commit
    #if ! git -c user.name="Kubernetes Publisher" -c user.email="k8s-publish-robot@users.noreply.github.com" rebase --onto ${oldHEAD} ${removeVendorHEAD} HEAD; then
    #	echo
    #    git diff -a --cc | sed 's/^/    /'
    #    echo
    #    git status | sed 's/^/    /'
    #    return 1
    #fi

    # sanity check whether our new branch actually contains the old tip
    git branch --contains ${oldHEAD}
}

# This function updates the vendor/ folder, and removes k8s.io/* and glog from 
# the vendor/ if the repo being published is a library. This is to avoid issues
# like https://github.com/kubernetes/client-go/issues/83 and
# https://github.com/kubernetes/client-go/issues/19.
#
# "deps" lists the dependent k8s.io/* repos and branches. For example, if the
# function is handling the release-1.6 branch of k8s.io/apiserver, deps is
# expected to be "apimachinery:release-1.6,client-go:release-3.0". Dependencies
# are expected to be separated by ",", and the name of the dependent repo and
# the branch name are expected to be separated by ":".
#
# "is_library" indicates if the repo being published is a library.
#
# This function should be run after the Godeps.json are updated with the latest
# revisions of k8s.io/* dependencies.
#
# To avoid repeated godep restore, repositories should share the GOPATH.
#
# This function assumes to be called at the root of the repository that's going to be published.
# This function assumes the branch that need update is checked out.
# This function assumes it's the last step in the publishing process that's going to generate commits.
restore_vendor() {
    local deps="${1:-""}"
    IFS=',' read -a deps <<< "${DEPS}"
    dep_count=${#deps[@]}
    # The Godeps.json of apiserver, kube-aggregator, sample-apiserver in staging
    # don't contain entries for k8s.io/* repos, so we need to explicitly check
    # out a revision of deps.
    for (( i=0; i<${dep_count}; i++ )); do
        pushd ../"${deps[i]%%:*}"
            git checkout "${deps[i]##*:}"
        popd
    done

    local is_library="${2}"
    # At this step, currently only client-go's Godeps.json contains entries for
    # k8s.io repos, with commit hash of the first commit in the master branch.
    godep restore
    # need to remove the Godeps folder, otherwise godep won't save source code to vendor/
    rm -rf ./Godeps
    # otherwise `godep save` might fail, see https://github.com/kubernetes/test-infra/issues/2684
    rm -rf ./vendor
    godep save ./...
    if [ "${is_library}" = "true" ]; then
        echo "remove k8s.io/*, gofuzz, and glog from vendor/"
        # glog uses global variables, it panics when multiple copies are compiled.
        rm -rf ./vendor/github.com/golang/glog
        # this ensures users who get the repository via `go get` won't end up with
        # multiple copies of k8s.io/ repos. The only copy will be the one in the
        # GOPATH.
        # Godeps.json has a complete, up-to-date list of dependencies, so
        # Godeps.json will be the ground truth for users using godep/glide/dep.
        rm -rf ./vendor/k8s.io
        # see https://github.com/kubernetes/kubernetes/issues/45693
        rm -rf ./vendor/github.com/google/gofuzz
    fi
    git add --all
    # check if there are new contents 
    if git diff --cached --exit-code &>/dev/null; then
        echo "vendor hasn't changed!"
        return
    fi
    git -c user.name="Kubernetes Publisher" -c user.email="k8s-publish-robot@users.noreply.github.com" commit -m "sync: resync vendor folder"
}

# find the rev when the given commit was merged into the branch
function git-find-merge() {
    { git rev-list ${1}..${2:-master} --ancestry-path; git rev-parse ${1}; } | grep -f <(git rev-list ${1}^1..${2:-master} --first-parent) | tail -1
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

# Godeps.json copied from the staging area might contain invalid commit hashes
# in entries for k8s.io/*. This function updates entry for k8s.io/${1} to track
# the latest commit created by the publishing robot.
# Currently this function is only useful for client-go. Entries for k8s.io/* are
# removed from the Godeps.json in the staging area of other repos.
update_godeps_json() {
    local repo=${1%%:*}
    local branch=${1##*:}
    local godeps_json="./Godeps/Godeps.json"
    local old_revs=""
    # TODO: pass in the new_rev if we want to depend on a specific revision.
    local new_rev=$(cd ../${repo}; git rev-parse ${branch})

    # TODO: simplify the following lines
    while read path rev; do
        if [[ "${path}" == "k8s.io/${repo}"* ]]; then
            old_revs+="${rev}"$'\n'
        fi
    done < <(jq '.Deps|.[]|.ImportPath + " " + .Rev' -r < "${godeps_json}")
    old_revs=$(echo "${old_revs%%$'\n'}" | sort | uniq)
    while read old_rev; do
        if [[ -z "${old_rev}" ]]; then
            continue
        fi
        sed -i "s|${old_rev}|${new_rev}|g" "${godeps_json}"
    done <<< "${old_revs}"
}
