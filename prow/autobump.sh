#!/bin/bash
# Copyright 2018 The Kubernetes Authors.
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

# bump.sh will
# * ensure there are no pending changes
# * optionally activate GOOGLE_APPLICATION_CREDENTIALS and configure-docker if set
# * run //prow:release-push to build and push prow images
# * update all the cluster/*.yaml files to use the new image tags

set -o errexit
set -o nounset
set -o pipefail

# TODO(fejta): rewrite this in a better language REAL SOON

# See https://misc.flogisoft.com/bash/tip_colors_and_formatting

color-version() { # Bold blue
  echo -e "\x1B[1;34m${@}\x1B[0m"
}

cd "$(dirname "${BASH_SOURCE}")"

if [[ $# -lt 2 ]]; then
    echo "Usage: $(basename "$0") <github-login> </path/to/github/token> [git-name] [git-email]" >&2
    exit 1
fi
user=$1
token=$2
shift
shift
ensure-config() {
  if [[ $# -eq 2 ]]; then
    echo "git config user.name=$1 user.email=$2..." >&2
    git config user.name "$1"
    git config user.email "$2"
  fi
  git config user.name &>/dev/null && git config user.email &>/dev/null && return 0
  echo "ERROR: git config user.name, user.email unset. No defaults provided" >&2
  return 1
}
ensure-config "$@"

echo "Bumping prow to latest..." >&2
./bump.sh --latest

# Convert image: gcr.io/k8s-prow/plank:v20181122-abcd to v20181122-abcd
extract-version() {
  local v=$(grep plank:v "$@")
  echo ${v##*plank:}
}

# Convert v20181111-abcd to abcd
extract-commit() {
  local c=$1
  echo ${c##*-}
}

old_version=$(git show HEAD:prow/cluster/plank_deployment.yaml | extract-version)
version=$(cat cluster/plank_deployment.yaml | extract-version)
comparison=$(extract-commit "$old_version")...$(extract-commit "$version")
echo -e "Pushing $(color-version $version) to ${user}:autobump..." >&2

title="Bump prow from $old_version to $version"
git add -A
git commit -m "$title"
git push -f "git@github.com:${user}/test-infra.git" HEAD:autobump

echo "Creating PR to merge ${user}:autobump into master..." >&2
bazel run //robots/pr-creator -- \
    --github-token-path="$token" \
    --org=kubernetes --repo=test-infra --branch=master \
    --title="$title" --match-title="Bump prow to" \
    --body="Included changes: https://github.com/kubernetes/test-infra/compare/$comparison" \
    --source="${user}":autobump \
    --confirm
