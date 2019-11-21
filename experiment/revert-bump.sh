#!/usr/bin/env bash
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

bold_blue() {
  echo -e "\x1B[1;34m${*}\x1B[0m"
}

bold_yellow() {
  echo -e "\x1B[1;33m${*}\x1B[0m"
}

main() {
  bold_blue "Checking out latest upstream ..."
  git fetch https://github.com/kubernetes/test-infra master
  git checkout FETCH_HEAD

  bold_blue "Finding most recent bump commit ..."
  local most_recent_bump
  local most_recent_bump_commmit
  most_recent_bump="$(git log --no-merges --extended-regexp \
    --grep='Update prow to v[a-f0-9]+-[a-f0-9]+, and other images as necessary\.' \
    --pretty=oneline -n 1)"
  bold_yellow "Matching Commit: ${most_recent_bump}"
  most_recent_bump_commmit=$(cut -f1 -d ' ' <<< "${most_recent_bump}")
  echo "Commit SHA: ${most_recent_bump_commmit}"

  bold_blue "Creating revert ..."
  local revert_branch
  revert_branch="revert-$(date +'%s')-${most_recent_bump_commmit}"
  git checkout -b "${revert_branch}"
  git revert --no-edit "${most_recent_bump_commmit}"

  bold_blue "Pushing branch ..."
  git -c push.default=current push

  bold_yellow "You should now file a pull request from your ${revert_branch} branch."
}

main "$@"
