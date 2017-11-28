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
set -o xtrace

# sync_repo() cherry picks the latest changes in k8s.io/kubernetes/<repo> to the
# local copy of the repository to be published.
#
# Prerequisites
# =============
#
# 1. we are in the root of the repository to be published
# 2. we are on the branch to be published (let's call it "destination branch"), possibly this
#    is an orphaned branch for new branches.
#
# Overall Flow
# ============
#
# 1. Fetch the current level of k8s.io/kubernetes.
# 2. Check out the ${src_branch} of k8s.io/kubernetes as branch filter-branch.
# 3. Rewrite the history of branch filter-branch to *only* include code in ${subdirectory},
#    keeping the original, corresponding k8s.io/kubernetes commit hashes
#    via "Kubernetes-commit: <kube-sha>" lines in the commit messages.
# 4. Locate all commits between the last time we sync'ed and now on the mainline, i.e. using
#    first-parent ancestors only, not going into a PR branch.
# 5. Switch back to the ${dst_branch}
# 6. For each commit C on the mainline (= ${f_mainline_commit} in the code) identified in 4:
#    a) If C is a merge commit:
#       i)   Find the latest branching point B from the mainline leading to C as its second parent.
#       ii)  If there is another merge between B and C via the path of (i), apply the diff between
#            the latest branching point B and the last merge M on that path.
#       iii) Continue cherry-picking all single commits between B (if (ii) didn’t apply) or M
#            (by (ii) these are no merges).
#       iv)  Commit the merge commit C as empty fast-forward merge of the latest branching point
#            and the HEAD produced from of (ii) and (iii).
#    b) If C is not a merge commit:
#       i)   Find the corresponding k8s.io/kubernetes commit k_C and find its merge point k_M
#            with the mainline in k8s.io/kubernetes.
#       ii)  Cherry-pick k_C.
#       iii) Continue with the next C in 6, let’s call it C’. If
#            - there is no such C’  or
#            - C’ is a merge commit or
#            - the mainline merge point k_M’ of the corresponding k8s.io/kubernetes commit k_C’
#              is different from k_M,
#            cherry-pick k_M as fast-forward merge commit of the mainline and C.
#
# Dropped PR Merges
# =================
#
# The logic of 6b is necessary because git filter-branch drops fast-forward merges, leading to
# a picture like in the next section..
#
# With 6b we get a perfectly uniform sequence of branches and fast-forward merges, i.e. without
# any interleaving. For this to work it is essential that Github’s merge commit are clean as
# described above.
#
# Result
# ======
#
# This gives the following picture. All merges on the mainline are fast-forward merges.
# All branches of the second parents are linear.
#
#   M─┐ [master] Merge pull request #51467 from liggitt/client-go-owner
#   │ o Add liggitt to client-go approvers
#   │─┘
#   M─┐ Merge pull request #50562 from atlassian/call-cleanup-properly
#   │ o Call the right cleanup function
#   │─┘
#   M─┐ Merge pull request #51154 from RenaudWasTaken/gRPC-updated-1-3-0
#   │ o Bumped gRPC version to 1.3.0
#   │ o sync: reset Godeps/Godeps.json
#   │─┘
#
# Master Merges
# =============
#
# The is a special case that a merge on the mainline of a non-master branch is a merge with
# the master branch. In that case, this master-merge is recreated on the ${dst_branch}
# pointing to the dst master branch with the second parent:
#
#   M─┐ {release-5.0} Merge remote-tracking branch 'origin/master' into release-1.8
#   │ M─┐ {master} Merge pull request #51876 from smarterclayton/disable_client_paging
#   │ │ o Disable default paging in list watches
#   o │ │ Kubernetes version v1.8.0-beta.1 file updates
#   M─┤ │ Merge remote-tracking branch 'origin/master' into release-1.8
#   │ │─┘
#   │ M─┐ Merge pull request #50708 from DirectXMan12/versions/autoscaling-v2beta1
#   │ │ o Move Autoscaling v2{alpha1 --> beta1}
#   │ │─┘
#   │ I─┐ Merge pull request #51795 from dims/bug-fix-51755
#   │ │ o Bug Fix - Adding an allowed address pair wipes port security groups
#   │ │ o sync: reset Godeps/Godeps.json
#   │ │─┘
#
# Code Conventions
# ================
#
# 1. variables prefixed with
#    - k_ refer to k8s.io/kubernetes
#    - f_ refer to the filtered-branch, i.e. rewritten using filtered-branch
#    - dst_ refer to the published repository.
# 2. there are the functions
#    - kube-commit to map a f_..._commit or dst_..._commit to the corresponding
#      k_..._commmit using the "Kubernetes-commit: <sha>" line in the commit message,
#    - branch-commit to map a k_..._commit to a f_..._commit or dst_..._commit
#      (depending on the current branch or the second paramter if given.
sync_repo() {
    # subdirectory in k8s.io/kubernetes, e.g., staging/src/k8s.io/apimachinery
    local subdirectory="${1}"
    local src_branch="${2}"
    local dst_branch="${3}"
    local kubernetes_remote="${4:-https://github.com/kubernetes/kubernetes.git}"
    local deps="${5:-""}"
    local is_library="${6}"
    readonly subdirectory src_branch dst_branch kubernetes_remote deps is_library

    local new_branch="false"
    local orphan="false"
    if ! git rev-parse -q --verify HEAD; then
        echo "Found repo without ${dst_branch} branch, creating initial commit."
        git commit -m "Initial commit" --allow-empty
        new_branch="true"
        orphan="true"
    elif [ $(ls -1 | wc -l) = 0 ]; then
        echo "Found repo without files, assuming it's new."
        new_branch="true"
    else
        echo "Starting at existing ${dst_branch} commit $(git rev-parse HEAD)."
    fi

    # fetch upstream kube and checkout $src_branch, name it filtered-branch
    git remote rm upstream-kube >/dev/null || true
    git remote add upstream-kube "${kubernetes_remote}" >/dev/null
    git fetch -q upstream-kube --no-tags
    git branch -D filtered-branch >/dev/null || true
    git branch -f upstream-branch upstream-kube/"${src_branch}"
    echo "Checked out k8s.io/kubernetes commit $(git rev-parse upstream-branch)."
    git checkout -q upstream-branch -b filtered-branch
    git reset -q --hard upstream-branch

    # filter filtered-branch (= ${src_branch}) by ${subdirectory} modifying commits
    # and rewrite paths. Each filtered commit (which is not dropped), gets the
    # original k8s.io/kubernetes commit hash in the commit message as "Kubernetes-commit: <hash>".
    # Then select all new mainline commits on filtered-branch as ${f_mainline_commits}
    # to loop through them later.
    local f_mainline_commits=""
    if [ "${new_branch}" = "true" ] && [ "${src_branch}" = master ]; then
        # new master branch
        filter-branch "${subdirectory}" ${src_branch} filtered-branch

        # find commits on the main line (will mostly be merges, but could be non-merges if filter-branch dropped
        # the corresponding fast-forward merge and left the feature branch commits)
        f_mainline_commits=$(git log --first-parent --format='%H' --reverse HEAD)

        # create and checkout new, empty master branch. We only need this non-orphan case for the master
        # as that usually exists for new repos.
        if [ ${orphan} = true ]; then
            git checkout -q ${dst_branch} --orphan
        else
            git checkout -q ${dst_branch}
        fi
    else
        # create filtered-branch-base before filtering for
        # - new branches that branch off master (i.e. the branching point)
        # - old branch which continue with the last old commit.
        if [ "${new_branch}" = "true" ]; then
            # new non-master branch
            local k_branch_point_commit=$(git-fork-point upstream-kube/${src_branch} upstream-kube/master)
            if [ -z "${k_branch_point_commit}" ]; then
                echo "Couldn't find a branch point of upstream-kube/${src_branch} and upstream-kube/master."
                return 1
            fi
            echo "Using branch point ${k_branch_point_commit} as new starting point for new branch ${dst_branch}."
            git branch -f filtered-branch-base ${k_branch_point_commit} >/dev/null

            echo "Rewriting upstream branch ${src_branch} to only include commits for ${subdirectory}."
            filter-branch "${subdirectory}" filtered-branch filtered-branch-base

            # for a new branch that is not master: map filtered-branch-base to our ${dst_branch} as ${dst_branch_point_commit}
            local k_branch_point_commit=$(kube-commit filtered-branch-base) # k_branch_point_commit will probably different thanthe k_branch_point_commit
                                                                            # above because filtered drops commits and maps to ancestors if necessary
            local dst_branch_point_commit=$(branch-commit ${k_branch_point_commit} master)
            if [ -z "${dst_branch_point_commit}" ]; then
                echo "Couldn't find a corresponding branch point commit for ${k_branch_point_commit} as ascendent of origin/master."
                return 1
            fi

            git branch -f ${dst_branch} ${dst_branch_point_commit} >/dev/null
        else
            # old branch
            local k_base_commit="$(last-kube-commit ${dst_branch} || true)"
            if [ -z "${k_base_commit}" ]; then
                echo "Couldn't find a Kubernetes-commit sha in any commit on ${dst_branch}."
                return 1
            fi
            local k_base_merge=$(git-find-merge ${k_base_commit} upstream-kube/${src_branch})
            if [ -z "${k_base_merge}" ]; then
                echo "Didn't find merge commit of k8s.io/kubernetes commit ${k_base_commit}. Odd."
                return 1
            fi
            git branch -f filtered-branch-base ${k_base_merge} >/dev/null

            echo "Rewriting upstream branch ${src_branch} to only include commits for ${subdirectory}."
            filter-branch "${subdirectory}" filtered-branch filtered-branch-base
        fi

        # find commits on the main line (will mostly be merges, but could be non-merges if filter-branch dropped
        # the corresponding fast-forward merge and left the feature branch commits)
        local f_base_commit=$(git rev-parse filtered-branch-base)
        f_mainline_commits=$(git log --first-parent --format='%H' --reverse ${f_base_commit}..HEAD)

        # checkout our dst branch. For old branches this is the old HEAD, for new non-master branches this is branch point on master.
        echo "Checking out branch ${dst_branch}."
        git checkout -q ${dst_branch}
    fi

    # remove old kubernetes-sha
    # TODO: remove once we are sure that no branches with kubernetes-sha exist anymore
    if [ -f kubernetes-sha ]; then
        git rm -q kubernetes-sha
        git commit -q -m "sync: remove kubernetes-sha"
    fi

    local dst_old_head=$(git rev-parse HEAD) # will be the initial commit for new branch

    # apply all PRs
    local k_pending_merge_commit=""
    local dst_needs_godeps_update=${new_branch} # has there been a godeps reset which requires a complete godeps update?
    local dst_merge_point_commit=$(git rev-parse HEAD) # the ${dst_branch} HEAD after the last applied f_mainline_commit
    for f_mainline_commit in ${f_mainline_commits} FLUSH_PENDING_MERGE_COMMIT; do
        local k_mainline_commit=""
        local k_new_pending_merge_commit=""

        if [ ${f_mainline_commit} = FLUSH_PENDING_MERGE_COMMIT ]; then
            # enforce that the pending merge commit is flushed
            k_new_pending_merge_commit=FLUSH_PENDING_MERGE_COMMIT
        else
            k_mainline_commit=$(kube-commit ${f_mainline_commit})

            # check under which merge with the mainline ${k_mainline_commit}) is
            k_new_pending_merge_commit=$(git-find-merge ${k_mainline_commit} upstream-branch)
            if [ "${k_new_pending_merge_commit}" = "${k_mainline_commit}" ]; then
                # it's on the mainline itself, no merge above it
                k_new_pending_merge_commit=""
            fi
            if [ ${dst_branch} != master ] && is-merge-with-master "${k_mainline_commit}"; then
                # merges with master on non-master branches we always handle as pending merge commit.
                k_new_pending_merge_commit=${k_mainline_commit}
            fi
        fi
        if [ -n "${k_pending_merge_commit}" ] && [ "${k_new_pending_merge_commit}" != "${k_pending_merge_commit}" ]; then
            # the new pending merge commit is different than the old one. Apply the old one. Three cases:
            # a) it's a merge with master on a non-master branch
            #    (i) it's on the filtered-branch
            #    (ii) it's dropped on the filtered-branch, i.e. fast-forward
            # b) it's another merge
            local dst_parent2="HEAD"
            if [ ${dst_branch} != master ] && is-merge-with-master "${k_pending_merge_commit}"; then
                # it's a merge with master. Recreate this merge on ${dst_branch} with ${dst_parent2} as second parent on the master branch
                local k_parent2="$(git rev-parse ${k_pending_merge_commit}^2)"
                read k_parent2 dst_parent2 <<<$(look -b ${k_parent2} ../kube-commits-$(basename "${PWD}")-master)
                if [ -z "${dst_parent2}" ]; then
                    echo "Corresponding k8s.io/$(dirname ${PWD}) master branch commit not found for upstream master merge ${k_pending_merge_commit}. Odd."
                    return 1
                fi

                f_pending_merge_commit=$(branch-commit ${k_pending_merge_commit} filtered-branch)
                if [ -n "${f_pending_merge_commit}" ]; then
                    echo "Cherry-picking k8s.io/kubernetes master-merge  ${k_pending_merge_commit}: $(commit-subject ${k_pending_merge_commit})."

                    # cherry-pick the difference on the filtered mainline
                    reset-godeps ${f_pending_merge_commit}^1 # unconditionally reset godeps
                    dst_needs_godeps_update=true
                    if ! GIT_COMMITTER_DATE="$(commit-date ${f_pending_merge_commit})" git cherry-pick --keep-redundant-commits -m 1 ${f_pending_merge_commit} >/dev/null; then
                        echo
                        show-working-dir-status
                        return 1
                    fi
                    squash 2
                else
                    # the merge commit with master was dropped. This means it was a fast-forward merge,
                    # which means we can just re-use the tree on the dst master branch.
                    echo "Cherry-picking k8s.io/kubernetes dropped-master-merge ${k_pending_merge_commit}: $(commit-subject ${k_pending_merge_commit})."
                    git reset -q --hard ${dst_parent2}
                fi
            else
                echo "Cherry-picking k8s.io/kubernetes dropped-merge ${k_pending_merge_commit}: $(commit-subject ${k_pending_merge_commit})."
            fi
            local date=$(commit-date ${k_pending_merge_commit}) # author and committer date is equal for PR merges
            local dst_new_merge=$(GIT_COMMITTER_DATE="${date}" GIT_AUTHOR_DATE="${date}" git commit-tree -p ${dst_merge_point_commit} -p ${dst_parent2} -m "$(commit-message ${k_pending_merge_commit}; echo; echo "Kubernetes-commit: ${k_pending_merge_commit}")" HEAD^{tree})
            # no amend-godeps needed here: because the merge-commit was dropped, both parents had the same tree, i.e. Godeps.json did not change.
            git reset -q --hard ${dst_new_merge}
            fix-godeps "${deps}" "${is_library}" ${dst_needs_godeps_update}
            dst_needs_godeps_update=false
            dst_merge_point_commit=$(git rev-parse HEAD)
        fi
        k_pending_merge_commit="${k_new_pending_merge_commit}"

        # stop the loop?
        if [ ${f_mainline_commit} = FLUSH_PENDING_MERGE_COMMIT ]; then
            break
        fi

        # is it a merge or a single commit on the mainline to apply?
        if [ ${dst_branch} != master ] && is-merge-with-master ${k_mainline_commit}; then
            echo "Deferring master merge commit ${k_mainline_commit}: $(commit-subject ${f_mainline_commit})."
        elif [ ${dst_branch} != master ] && [ -n "${k_pending_merge_commit}" ] && is-merge-with-master "${k_pending_merge_commit}"; then
            echo "Skipping master commit ${k_mainline_commit}: $(commit-subject ${f_mainline_commit}). Master merge commit ${k_pending_merge_commit} is pending."
        elif ! git show -q ${f_mainline_commit} | grep -q "^Merge: " || pick-merge-as-single-commit ${k_mainline_commit}; then
            local pick_args=""
            if git show -q ${f_mainline_commit} | grep -q "^Merge: "; then
                pick_args="-m 1"
                echo "Cherry-picking k8s.io/kubernetes merge-commit  ${k_mainline_commit}: $(commit-subject ${f_mainline_commit})."
            else
                echo "Cherry-picking k8s.io/kubernetes single-commit ${k_mainline_commit}: $(commit-subject ${f_mainline_commit})."
            fi

            # reset Godeps.json?
            local squash_commits=1
            if godep-changes ${f_mainline_commit}; then
                reset-godeps ${f_mainline_commit}^
                squash_commits=2 # squash the cherry-pick into the godep reset commit below
                dst_needs_godeps_update=true
            fi

            # finally cherry-pick
            if ! GIT_COMMITTER_DATE="$(commit-date ${f_mainline_commit})" git cherry-pick --keep-redundant-commits ${pick_args} ${f_mainline_commit} >/dev/null; then
                echo
                show-working-dir-status
                return 1
            fi

            # potentially squash godep reset commit
            squash ${squash_commits}

            # if there is no pending merge commit, update Godeps.json because this could be a target of tag
            if [ -z "${k_pending_merge_commit}" ]; then
                fix-godeps "${deps}" "${is_library}" ${dst_needs_godeps_update}
                dst_needs_godeps_update=false
                dst_merge_point_commit=$(git rev-parse HEAD)
            fi
        else
            # find **latest** (in the sense of least distance to ${f_mainline_commit}) common ancestor of both parents
            # of ${f_mainline_commit}. If the PR had no merge commits, this is the actual fork point. If there have been
            # merges, we will get the latest of those merges. Everything between that and ${f_mainline_commit} will be
            # linear. This will potentially drop commit history, but no actual changes.
            #
            # The simple merge case:                       The interleaved merge case:
            #
            #   M─┐ f_mainline_commit                        M───┐ f_mainline_commit
            #   │ o f_mainline_commit^2                      │   o f_mainline_commit^2
            #   o │ f_mainline_commit^1                      o   │ f_mainline_commit^1
            #   o │                                          M─┐ │
            #   │─M f_latest_merge_commit                    │ o │
            #   o │ f_latest_branch_point_commit             │ │─M f_latest_merge_commit
            #   │─M <--- some older merge                    │ o │ f_latest_branch_point_commit
            #   │ o <--- lost from the history               │─┘ o <--- lost from the history
            #   │─┘                                          │───M <--- some older merge
            #   │                                            o   │
            #                                                │───┘
            #                                                │
            local f_latest_branch_point_commit=$(git merge-base --octopus ${f_mainline_commit}^1 ${f_mainline_commit}^2)
            if [ -z "${f_latest_branch_point_commit}" ]; then
                echo "No branch point found for PR merged through ${f_mainline_commit}. Odd."
                return 1
            fi

            # start cherry-picking with latest merge, squash everything before.
            # Note: we want to ban merge commits on feature branches, compare https://github.com/kubernetes/kubernetes/pull/51176.
            #       Until that or something equivalent is in place, we do best effort here not to fall over those merge commits.
            local f_first_pick_base=${f_latest_branch_point_commit}
            local f_latest_merge_commit=$(git log --merges --format='%H' --ancestry-path -1 ${f_latest_branch_point_commit}..${f_mainline_commit}^2)
            if [ -n "${f_latest_merge_commit}" ]; then
                echo "Cherry-picking squashed k8s.io/kubernetes branch-commits $(kube-commit ${f_latest_branch_point_commit})..$(kube-commit ${f_latest_merge_commit}) because the last one is a merge: $(commit-subject ${f_latest_merge_commit})"

                # reset Godeps.json?
                local squash_commits=1
                if godep-changes ${f_latest_branch_point_commit} ${f_latest_merge_commit}; then
                    reset-godeps ${f_latest_branch_point_commit}
                    squash_commits=2 # squash the cherry-pick into the godep reset commit below
                    dst_needs_godeps_update=true
                fi

                if ! git diff ${f_latest_branch_point_commit} ${f_latest_merge_commit} | git apply --index; then
                    echo
                    show-working-dir-status
                    return 1
                fi
                git commit -q -m "sync: squashed up to merge $(kube-commit ${f_latest_merge_commit}) in ${k_mainline_commit}" --date "$(commit-date ${f_latest_merge_commit})" --author "$(commit-author ${f_latest_merge_commit})"
                ensure-clean-working-dir

                # potentially squash godep reset commit
                squash ${squash_commits}

                # we start cherry-picking now from f_latest_merge_commit up to the actual Github merge into the mainline
                f_first_pick_base=${f_latest_merge_commit}
            fi
            for f_commit in $(git log --format='%H' --reverse ${f_first_pick_base}..${f_mainline_commit}^2); do
                # reset Godeps.json?
                local squash_commits=1
                if godep-changes ${f_commit}; then
                    reset-godeps ${f_commit}^
                    squash_commits=2 # squash the cherry-pick into the godep reset commit below
                    dst_needs_godeps_update=true
                fi

                echo "Cherry-picking k8s.io/kubernetes branch-commit $(kube-commit ${f_commit}): $(commit-subject ${f_commit})."
                if ! GIT_COMMITTER_DATE="$(commit-date ${f_commit})" git cherry-pick --keep-redundant-commits ${f_commit} >/dev/null; then
                    echo
                    show-working-dir-status
                    return 1
                fi
                ensure-clean-working-dir

                # potentially squash godep reset commit
                squash ${squash_commits}
            done

            # commit empty PR merge. This will carry the actual SHA1 from the upstream commit. It will match tags as well.
            echo "Cherry-picking k8s.io/kubernetes branch-merge  ${k_mainline_commit}: $(commit-subject ${f_mainline_commit})."
            local date=$(commit-date ${f_mainline_commit}) # author and committer date is equal for PR merges
            git reset -q $(GIT_COMMITTER_DATE="${date}" GIT_AUTHOR_DATE="${date}" git commit-tree -p ${dst_merge_point_commit} -p HEAD -m "$(commit-message ${f_mainline_commit})" HEAD^{tree})

            # reset to mainline state which is guaranteed to be correct.
            # On the feature branch we might have reset to an too early state:
            #
            # In k8s.io/kubernetes:              Linearized in published repo:
            #
            #   M───┐ f_mainline_commit, result B  M─┐ result B
            #   M─┐ │ result A                     │ o change B
            #   │ o │ change A                     │─┘
            #   │ │ o change B                     M─┐ result A
            #   │─┘ │ base A                       │ o change A
            #   │───┘ base B                       │─┘
            #
            # Compare that with amending f_mainline_commit's Godeps.json into the HEAD,
            # we get result B in the linearized version as well. In contrast with this,
            # we would end up with "base B + change B" which misses the change A changes.
            amend-godeps-at ${f_mainline_commit}

            fix-godeps "${deps}" "${is_library}" ${dst_needs_godeps_update}
            dst_needs_godeps_update=false
            dst_merge_point_commit=$(git rev-parse HEAD)
        fi

        ensure-clean-working-dir
    done

    # get consistent and complete godeps on each sync. Skip if nothing changed.
    # NOTE: we cannot skip collapsed-kube-commit-mapper below because its
    #       output depends on upstream's HEAD.
    if [ $(git rev-parse HEAD) != "${dst_old_head}" ] || [ "${new_branch}" = "true" ]; then
        fix-godeps "${deps}" "${is_library}" true
    fi

    # create look-up file for collapsed upstream commits
    local repo=$(basename ${PWD})
    if [ -n "$(git log --oneline --first-parent --merges | head -n 1)" ]; then
        echo "Writing k8s.io/kubernetes commit lookup table to ../kube-commits-${repo}-${dst_branch}"
        /collapsed-kube-commit-mapper --upstream-branch refs/heads/upstream-branch > ../kube-commits-${repo}-${dst_branch}
    else
        echo "No merge commit on ${dst_branch} branch, must be old. Skipping look-up table."
        echo > ../kube-commits-${repo}-${dst_branch}
    fi
}

# for some PR branches cherry-picks fail. Put commits here where we only pick the whole merge as a single commit.
function pick-merge-as-single-commit() {
    grep -F -q -x "$1" <<EOF
25ebf875b4235cb8f43be2aec699d62e78339cec
EOF
}

# amend-godeps-at checks out the Godeps.json at the given commit and amend it to the previous commit.
function amend-godeps-at() {
    if [ -f Godeps/Godeps.json ]; then
        git checkout ${f_mainline_commit} Godeps/Godeps.json # reset to mainline state which is guaranteed to be correct
        git commit --amend --no-edit -q
    fi
}

function commit-date() {
    git show --format="%aD" -q ${1}
}

function committer-date() {
    git show --format="%cD" -q ${1}
}

function commit-author() {
    git show --format="%an <%ae>" -q ${1}
}

function commit-message() {
    git show --format="%B" -q ${1}
}

function commit-subject() {
    git show --format="%s" -q ${1}
}

# rewrites git history to *only* include $subdirectory
function filter-branch() {
    local subdirectory="${1}"
    shift
    git filter-branch -f --msg-filter 'awk 1 && echo && echo "Kubernetes-commit: ${GIT_COMMIT}"' --subdirectory-filter "${subdirectory}" -- "$@"
}

function is-merge-with-master() {
    if ! grep -q "^Merge remote-tracking branch 'origin/master'" <<<"$(commit-message ${1})"; then
        return 1
    fi
}

function ensure-clean-working-dir() {
    if ! git diff HEAD --exit-code &>/dev/null; then
        echo "Expected clean git working dir. It's not:"
        show-working-dir-status
        return 1
    fi
}

function show-working-dir-status() {
    git diff -a --cc | sed 's/^/    /'
    echo
    git status | sed 's/^/    /'
}

function godep-changes() {
    if [ -n "${2:-}" ]; then
        ! git diff --exit-code --quiet ${1} ${2} -- Godeps/Godeps.json
    else
        ! git diff --exit-code --quiet ${1}^ ${1} -- Godeps/Godeps.json
    fi
}

function branch-commit() {
    git log --grep "Kubernetes-commit: ${1}" --format='%H' ${2:-HEAD}
}

function last-kube-commit() {
    git log --format="%B" ${1:-HEAD} | grep "^Kubernetes-commit: " | head -n 1 | sed "s/^Kubernetes-commit: //g"
}

function kube-commit() {
    commit-message ${1:-HEAD} | grep "^Kubernetes-commit: " | sed "s/^Kubernetes-commit: //g"
}

# find the rev when the given commit was merged into the branch
function git-find-merge() {
    # taken from https://stackoverflow.com/a/38941227: intersection of both files, with the order of the second
    awk 'NR==FNR{a[$1]++;next} a[$1] ' \
        <(git rev-list ${1}^1..${2:-master} --first-parent) \
        <(git rev-list ${1}..${2:-master} --ancestry-path; git rev-parse ${1}) \
    | tail -1
}

# find the first common commit on the first-parent mainline of two branches, i.e. the point where a fork was started.
# By considering only the mainline of both branches, this will handle merges between the two branches by skipping
# them in the search.
function git-fork-point() {
    # taken from https://stackoverflow.com/a/38941227: intersection of both files, with the order of the second
    awk 'NR==FNR{a[$1]++;next} a[$1] ' \
        <(git rev-list ${2:-master} --first-parent) \
        <(git rev-list ${1:-HEAD} --first-parent) \
    | head -1
}

function git-index-clean() {
    if git diff --cached --exit-code &>/dev/null; then
        return 0
    fi
    return 1
}

function fix-godeps() {
    local deps="${1:-""}"
    local is_library="${2}"
    local needs_godeps_update="${3}"
    local dst_old_commit=$(git rev-parse HEAD)
    if [ "${needs_godeps_update}" = true ]; then
        # run godeps restore+save
        update_full_godeps "${deps}" "${is_library}"
    elif [ -f Godeps/Godeps.json ]; then
        # update the Godeps.json quickly by just updating the dependency hashes
        # Note: this is a compromise between correctness and completeness. It's neither 100%
        #       of these, but good enough for go get and vendoring tools.
        checkout-deps-to-kube-commit "${deps}"
        update-deps-in-godep-json "${deps}" "${is_library}"
    fi

    # remove vendor/ on non-master branches for libraries
    if [ "$(git rev-parse --abbrev-ref HEAD)" != master ] && [ -d vendor/ ] && [ "${is_library}" = "true" ]; then
        echo "Removing vendor/ on non-master branch because this is a library"
        git rm -q -rf vendor/
        if ! git-index-clean; then
            git commit -q -m "sync: remove vendor/"
        fi
    fi

    # amend godep commit to ${dst_old_commit}
    if ! git diff --exit-code ${dst_old_commit} &>/dev/null; then
        echo "Amending last merge with godep changes."
        git reset --soft -q ${dst_old_commit}
        git commit -q --amend -C ${dst_old_commit}
    fi

    ensure-clean-working-dir
}

# Reset Godeps.json to what it looked like in the given commit $1. Always create a
# commit, even an empty one.
function reset-godeps() {
    local f_clean_commit=${1}

    # checkout or delete Godeps/Godeps.json
    if [ -n "$(git ls-tree ${f_clean_commit}^{tree} Godeps)" ]; then
        git checkout ${f_clean_commit} Godeps
        git add Godeps
    elif [ -d Godeps ]; then
        rm -rf Godeps
        git rm -rf Godeps
    fi

    # commit Godeps/Godeps.json unconditionally
    git commit -q -m "sync: reset Godeps/Godeps.json" --allow-empty
}

# Squash the last $1 commits into one, with the commit message of the last.
function squash() {
    local head=$(git rev-parse HEAD)
    git reset -q --soft HEAD~${1:-2}
    GIT_COMMITTER_DATE=$(committer-date ${head}) git commit --allow-empty -q -C ${head}
}

# This function updates vendor/ and Godeps/Godeps.json.
#
# "deps" lists the dependent k8s.io/* repos and branches. For example, if the
# function is handling the release-1.6 branch of k8s.io/apiserver, deps is
# expected to be "apimachinery:release-1.6,client-go:release-3.0". Dependencies
# are expected to be separated by ",", and the name of the dependent repo and
# the branch name are expected to be separated by ":".
#
# "is_library" indicates if the repo being published is a library.
#
# To avoid repeated godep restore, repositories should share the GOPATH.
#
# This function assumes to be called at the root of the repository that's going to be published.
# This function assumes the branch that need update is checked out.
# This function assumes it's the last step in the publishing process that's going to generate commits.
update_full_godeps() {
    local deps="${1:-""}"
    local is_library="${2}"

    ensure-clean-working-dir

    # clean up k8s.io/* checkouts. If any is dirty, we will fail here because godep restore is unhappy. This
    # can even include non-dependencies if the dependencies changed.
    for d in $../*; do
        if [ ! -d ${d} ]; then
            continue
        fi
        pushd ${d} >/dev/null
            git rebase --abort &>/dev/null || true
            git reset --hard -q
            git clean -f -f -d -q
        popd >/dev/null
    done

    if [ ! -f Godeps/Godeps.json ]; then
        echo "No Godeps/Godeps.json found. Skipping godeps completely until upstream adds it."
        return 0
    fi

    # remove dependencies from Godeps/Godeps.json
    echo "Removing k8s.io/* dependencies from Godeps.json"
    local dep=""
    local branch=""
    local depbranch=""
    for depbranch in ${deps//,/ } $(basename "${PWD}"); do # due to a bug in kube's update-staging-godeps script we have reflexive dependencies. Remove them as well.
        IFS=: read dep branch <<<"${depbranch}"
        jq '.Deps |= map(select(.ImportPath | (startswith("k8s.io/'${dep}'/") or . == "k8s.io/'${dep}'") | not))' Godeps/Godeps.json > Godeps/Godeps.json.clean
        mv Godeps/Godeps.json.clean Godeps/Godeps.json
    done

    echo "Running godep restore."
    godep restore

    # checkout k8s.io/* dependencies
    checkout-deps-to-kube-commit "${deps}"

    # recreate vendor/ and Godeps/Godeps.json
    rm -rf ./Godeps
    rm -rf ./vendor

    echo "Running godep save."
    godep save ./...

    # remove Comment from each dependency and use tabs
    jq 'del(.Deps[].Comment)' Godeps/Godeps.json | unexpand --first-only --tabs=2 > Godeps/Godeps.json.clean
    mv Godeps/Godeps.json.clean Godeps/Godeps.json

    if [ "${is_library}" = "true" ]; then
        if [ "$(git rev-parse --abbrev-ref HEAD)" != master ]; then
            echo "Removing complete vendor/ on non-master branch because this is a library."
            rm -rf vendor/
        else
            echo "Removing k8s.io/*, gofuzz, go-openapi and glog from vendor/ because this is a library."
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
            # go-openapi is shared between apiserver and apimachinery
            rm -rf ./vendor/github.com/go-openapi
        fi
    fi

    git add Godeps/Godeps.json
    git add vendor/ || true

    # check if there are new contents
    if git-index-clean; then
        echo "Godeps.json hasn't changed!"
    else
        echo "Committing vendor/ and Godeps/Godeps.json."
        git commit -q -m "sync: update godeps"
    fi

    # nothing should be left
    ensure-clean-working-dir
}

# update the dependencies to the version checked out
update-deps-in-godep-json() {
    if [ ! -f Godeps/Godeps.json ]; then
        return 0
    fi

    local is_library=${2}
    local deps=${1}
    local deps_array=()
    IFS=',' read -a deps_array <<< "${1}"
    local dep_count=${#deps_array[@]}
    for (( i=0; i<${dep_count}; i++ )); do
        local dep="${deps_array[i]%%:*}"
        local dep_commit=$(cd ../${dep}; git rev-parse HEAD)
        if [ -z "${dep_commit}" ]; then
            echo "Couldn't find kube commit for current HEAD. Odd."
            return 1
        fi

        # get old dependency hash in Godeps/Godeps.json
        local old_dep_commit=$(jq -r '.Deps[] | select(.ImportPath | startswith("k8s.io/'${dep}'/") or . == "k8s.io/'${dep}'") | .Rev' Godeps/Godeps.json | tail -n 1)
        if [ -n "${old_dep_commit}" ]; then
            if [ "${old_dep_commit}" != "${dep_commit}" ]; then
                # it existed before => replace with the new value
                echo "Updating k8s.io/${dep} dependency to ${dep_commit}."
                sed -i "s/${old_dep_commit}/${dep_commit}/g" Godeps/Godeps.json
            fi
        elif git grep -w -q "k8s.io/${dep}"; then
            # revert changes and fall back to full vendoring
            echo "Found new dependency k8s.io/${dep}. Switching to full vendoring."
            git checkout -q HEAD Godeps/Godeps.json
            update_full_godeps "${deps}" "${is_library}"
            return $?
        else
            echo "Ignoring k8s.io/${dep} dependency because it seems not to be used."
        fi
    done

    # due to a bug we have xxxx revisions for reflexive dependencies. Remove them.
    jq '.Deps |= map(select(.ImportPath | (startswith("k8s.io/'$(basename "${PWD}")'/") or . == "k8s.io/'$(basename "${PWD}")'") | not))' Godeps/Godeps.json > Godeps/Godeps.json.clean
    mv Godeps/Godeps.json.clean Godeps/Godeps.json

    git add Godeps/Godeps.json

    # check if there are new contents
    if git-index-clean; then
        echo "Godeps.json hasn't changed!"
    else
        echo "Committing Godeps/Godeps.json."
        git commit -q -m "sync: update godeps"
    fi

    # nothing should be left
    ensure-clean-working-dir
}

# checkout the dependencies to the versions corresponding to the kube commit of HEAD
checkout-deps-to-kube-commit() {
    local deps=()
    IFS=',' read -a deps <<< "${1}"

    # get last k8s.io/kubernetes commit on HEAD ...
    local k_last_kube_commit="$(last-kube-commit HEAD)"
    if [ -z "${k_last_kube_commit}" ]; then
        echo "No k8s.io/kubernetes commit found in the history of HEAD."
        return 1
    fi

    # ... and get possible merge point of it (in case of dropped fast-forward merges this
    # might have been dropped on HEAD).
    local k_last_kube_merge=$(git-find-merge "${k_last_kube_commit}" upstream-branch)

    local dep_count=${#deps[@]}
    for (( i=0; i<${dep_count}; i++ )); do
        local dep="${deps[i]%%:*}"
        local branch="${deps[i]##*:}"

        echo "Looking up which commit in the ${branch} branch of k8s.io/${dep} corresponds to k8s.io/kubernetes commit ${k_last_kube_merge}."
        local k_commit=""
        local dep_commit=""
        read k_commit dep_commit <<<$(look -b ${k_last_kube_merge} ../kube-commits-${dep}-${branch})
        if [ -z "${dep_commit}" ]; then
            echo "Could not find corresponding k8s.io/${dep} commit for kube commit ${k_last_kube_commit}."
            return 1
        fi

        pushd ../${dep} >/dev/null
            echo "Checking out k8s.io/${dep} to ${dep_commit}"
            git checkout -q "${dep_commit}"
        popd >/dev/null
    done
}
