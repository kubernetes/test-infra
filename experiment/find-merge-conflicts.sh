#!/usr/bin/env bash
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

# Usage: GITHUB_TOKEN=<api token> find-merge-conflicts.sh <ORG> <REPO>
#
# This script uses the GitHub API to find all open unmergable PRs (i.e. those
# with merge conflicts), then checks out the corresponding git repository and
# attempts to merge the PRs, printing conflicts to stdout.
#
# Status messages are printed to stderr with a hash (#) as prefix.
# Each PR is identified to stdout by a message prefixed with an asterisk (*).
# This makes it easy to process the results: stdout contains just the PRs and
# conflicts, and the PRs can be filtered out with "grep -v '^*'".
#
# For example, to save conflicts and a summarized tally:
#   GITHUB_TOKEN=<token> find-merge-conflicts.sh <ORG> <REPO> >conflicts.txt
#   grep -v '^*' conflicts.txt | sort | uniq -c | sort -rn >summary.txt

set -o errexit
set -o nounset
set -o pipefail

if [[ $# < 2 ]]; then
  echo "Usage: $0 <ORG> <REPO>" >&2
  exit 1
fi

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
  echo "GITHUB_TOKEN must be set." >&2
  exit 1
fi

readonly ORG=$1
readonly REPO=$2
readonly GHCURL=(curl -sfSLH "Authorization: token ${GITHUB_TOKEN}")

echo "# Searching for PRs with conflicts in ${ORG}/${REPO}" >&2
conflicting_prs=()

# This is pretty hacky.
page=0
while true; do
  page=$((page + 1))
  # Select only PRs that have not been tagged as stale or rotten.
  open_prs=$("${GHCURL[@]}" "https://api.github.com/repos/${ORG}/${REPO}/pulls?base=master&per_page=100&page=${page}" |\
    jq -r '.[] | select(.labels | map(.name != "lifecycle/stale" and .name != "lifecycle/rotten") | all) | .number')
  [[ -z "${open_prs}" ]] && break
  for pr in ${open_prs}; do
    mergeable=$("${GHCURL[@]}" -sfSL "https://api.github.com/repos/${ORG}/${REPO}/pulls/${pr}" | jq -r '.mergeable')
    if [[ "${mergeable}" == "false" ]]; then
      conflicting_prs+=("${pr}")
    fi
  done
done

if [[ -z "${conflicting_prs[@]:-}" ]]; then
  echo "# No PRs with conflicts found." >&2
  exit
fi

echo "# ${#conflicting_prs[@]} conflicting PRs found" >&2

tmpdir=$(mktemp -d)
trap "rm -rf ${tmpdir}" EXIT

echo "# Cloning git repo ${ORG}/${REPO}" >&2
git clone -q "https://github.com/${ORG}/${REPO}" "${tmpdir}"
cd "${tmpdir}"
git config merge.renameLimit 999999

echo "# Reporting conflicts" >&2
for pr in ${conflicting_prs[@]}; do
  # Create a branch to attempt the merge
  git branch -q merge-test master
  # Checkout that branch
  git checkout -q merge-test
  # Fetch the PR head
  echo "# Fetching refs/pull/${pr}/head" >&2
  git fetch -q -f origin "refs/pull/${pr}/head"
  # Merge it
  git merge --ff FETCH_HEAD --strategy resolve >/dev/null 2>/dev/null || true
  echo "* ${ORG}/${REPO}/pull/${pr}:"
  # Report files that are conflicting
  PAGER=cat git diff --name-only --diff-filter=UXB
  # Abort so we can reset
  git merge --abort >/dev/null 2>/dev/null || true
  git reset -q --hard
  git checkout -q master
  git branch -q -D merge-test
  git prune 2>/dev/null
done
