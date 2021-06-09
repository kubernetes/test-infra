#!/bin/bash

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

PROW_CONTROLLER_MANAGER_FILE="${PROW_CONTROLLER_MANAGER_FILE:-}"
# Args for use with GH
GH_ORG="${GH_ORG:-}"
GH_REPO="${GH_REPO:-}"
FORK_GH_REPO="${FORK_GH_REPO:-${GH_REPO}}"
# Args for use with Gerrit
GERRIT_HOST_REPO="${GERRIT_HOST_REPO:-}"

# The title prefix of the PR created by autobump.
# Omit this if you don't want to add a prefix.
AUTOBUMP_TITLE_PREFIX="${AUTOBUMP_TITLE_PREFIX:+"${AUTOBUMP_TITLE_PREFIX} "}"

# Set this to something more specific if the repo hosts multiple Prow instances.
# Must be a valid to use as part of a git branch name. (e.g. no spaces)
PROW_INSTANCE_NAME="${PROW_INSTANCE_NAME:-prow}"



# TODO(fejta): rewrite this in a better language REAL SOON  <-lol
main() {
	if [[ $# -lt 1 ]]; then
			echo "Usage: $(basename "$0") <path to github token or http cookiefile> [git-name] [git-email]" >&2
			return 1
	fi
	creds=$1
	shift
	check-args
	ensure-git-config "$@"

	echo "Bumping ${PROW_INSTANCE_NAME} to upstream (prow.k8s.io) version..." >&2
	/bump.sh --upstream

	cd "$(git rev-parse --show-toplevel)"
	old_version=$(git show "HEAD:${PROW_CONTROLLER_MANAGER_FILE}" | extract-version)
	version=$(cat "${PROW_CONTROLLER_MANAGER_FILE}" | extract-version)

	if [[ -z "${version}" ]]; then
		echo "Failed to fetch version from ${PROW_CONTROLLER_MANAGER_FILE}"
		exit 1
	fi
	if [[ "${old_version}" == "${version}" ]]; then
		echo "Bump did not change the Prow version: it's still ${version}. Aborting no-op bump." >&2
		return 0
	fi
	git add -u
	title="${AUTOBUMP_TITLE_PREFIX}Bump ${PROW_INSTANCE_NAME} from ${old_version} to ${version}"
	comparison=$(extract-commit "${old_version}")...$(extract-commit "${version}")
	body="Included changes: https://github.com/kubernetes/test-infra/compare/${comparison}"

	if [[ -n "${GH_ORG}" ]]; then
		create-gh-pr
	else
		create-gerrit-pr
	fi

	echo "autobump.sh completed successfully!" >&2
}

user-from-token() {
	user=$(curl -H "Authorization: token $(cat "${token}")" "https://api.github.com/user" 2>/dev/null | sed -n "s/\s\+\"login\": \"\(.*\)\",/\1/p")
}

ensure-git-config() {
	if [[ $# -eq 2 ]]; then
		echo "git config user.name=$1 user.email=$2..." >&2
		git config user.name "$1"
		git config user.email "$2"
	fi
	git config user.name &>/dev/null && git config user.email &>/dev/null && return 0
	echo "ERROR: git config user.name, user.email unset. No defaults provided" >&2
	return 1
}

check-args() {
	if [[ -z "${PROW_CONTROLLER_MANAGER_FILE}" ]]; then
		echo "ERROR: PROW_CONTROLLER_MANAGER_FILE must be specified." >&2
		return 1
	fi
	if [[ -z "${GERRIT_HOST_REPO}" ]]; then
		if [[ -z "${GH_ORG}" || -z "${GH_REPO}" ]]; then
			echo "ERROR: GH_ORG and GH_REPO must be specified to create a GitHub PR." >&2
			return 1
		fi
	else
		if [[ -n "${GH_ORG}" || -n "${GH_REPO}" ]]; then
			echo "ERROR: GH_ORG and GH_REPO cannot be used with GERRIT_HOST_REPO." >&2
			return 1
		fi
		if [[ -z "${GERRIT_HOST_REPO}" ]]; then
			echo "ERROR: GERRIT_HOST_REPO must be specified to create a Gerrit PR." >&2
			return 1
		fi
	fi
}

create-gh-pr() {
	git commit -m "${title}"

	token="${creds}"
	user-from-token
	
	echo -e "Pushing commit to github.com/${user}/${FORK_GH_REPO}:autobump-${PROW_INSTANCE_NAME}..." >&2
	git push -f "https://${user}:$(cat "${token}")@github.com/${user}/${FORK_GH_REPO}" "HEAD:autobump-${PROW_INSTANCE_NAME}" 2>/dev/null

	echo "Creating PR to merge ${user}:autobump-${PROW_INSTANCE_NAME} into master..." >&2
	/pr-creator \
		--github-token-path="${token}" \
		--org="${GH_ORG}" --repo="${GH_REPO}" --branch=master \
		--title="${title}" --head-branch="autobump-${PROW_INSTANCE_NAME}" \
		--body="${body}" \
		--source="${user}:autobump-${PROW_INSTANCE_NAME}" \
		--confirm
}

create-gerrit-pr() {
	git config http.cookiefile "${creds}"
	git remote add upstream "${GERRIT_HOST_REPO}"

	local change_id="$(get-change-id)"
	echo "Commit will have Change-Id: ${change_id}"

	# Check for an existing open PR and see if it needs to be updated.
	echo "Checking for an existing open Gerrit PR to update..."
	git fetch upstream "+refs/changes/*:refs/remotes/upstream/changes/*" 2> /dev/null
	local pr_commit="$(git log --all --grep="Change-Id: ${change_id}" -1 --format="%H")"
	if [[ -n "${pr_commit}" ]]; then
		local pr_version="$(git show "${pr_commit}:${PROW_CONTROLLER_MANAGER_FILE}" | extract-version)"
		if [[ "${pr_version}" == "${version}" ]]; then
			echo "Bump PR is already up to date (version ${version}). Aborting no-op update." >&2
			return 0
		fi
		echo "Bump PR is not up to date, it currently updates to ${pr_version}. Updating PR to ${version}."
	else
		echo "Did not find an existing PR to update. A new Gerrit PR will be created."
	fi

	git commit -m "${title}

${body}

Change-Id: ${change_id}"

	git push upstream HEAD:refs/for/master
}

# get-change-id generates a change ID for the gerrit PR that is deterministic
# rather than being random as is normally preferable.
# In particular this chooses a change ID that is unique to the host/repo,
# prow instance name, and the version we are bumping *from*. This ensures we
# update the existing open CL rather than opening a new one.
# HOWEVER, this doesn't work if there is a revert to a previous version since
# we will generated the change-id of an already merged PR. We avoid this by
# iteratively hashing the chosen ID until we find an unused ID.
get-change-id() {
	local id="I$(echo "${GERRIT_HOST_REPO}; ${PROW_INSTANCE_NAME}; ${old_version}" | git hash-object --stdin)"

	# While a commit on the base branch exists with this change ID...
	while [[ -n "$(git log --grep="Change-Id: ${id}" -F)" ]]; do
		# Choose another ID by hashing the current ID.
		id="I$(echo "${id}" | git hash-object --stdin)"
	done
	echo "${id}"
}

# Convert image: gcr.io/k8s-prow/plank:v20181122-abcd to v20181122-abcd
extract-version() {
	local v=$(grep prow-controller-manager:v "$@")
	echo ${v##*prow-controller-manager:}
}
# Convert v20181111-abcd to abcd
extract-commit() {
	local c=$1
	echo ${c##*-}
}

main "$@"
