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

# This script publishes the latest changes in the ${src_branch} of
# k8s.io/kubernetes/staging/src/${repo} to the ${dst_branch} of
# k8s.io/${repo}.
#
# dependent_k8s.io_repos are expected to be separated by ",", 
# e.g., "client-go,apimachinery". We will expand it to
# "repo:commit,repo:commit..." in the future.
#
# ${kubernetes_remote} is the remote url of k8s.io/kubernetes that will be used
# in .git/config in the local checkout of the ${repo}.
#
# is_library indicates is ${repo} is a library.
#
# The script assumes that the working directory is
# $GOPATH/src/k8s.io/${repo}.
#
# The script is expected to be run by other publish scripts.

set -o errexit
set -o nounset
set -o pipefail

if [ ! $# -eq 6 ]; then
    echo "usage: $0 repo src_branch dst_branch dependent_k8s.io_repos kubernetes_remote is_library"
    exit 1
fi

# the target repo
REPO="${1}"
# src branch of k8s.io/kubernetes
SRC_BRANCH="${2:-master}"
# dst branch of k8s.io/${repo}
DST_BRANCH="${3:-master}"
# dependent k8s.io repos
DEPS="${4}"
# Remote url for Kubernetes. If empty, will fetch kubernetes
# from https://github.com/kubernetes/kubernetes.
KUBERNETES_REMOTE="${5}"
# If ${REPO} is a library
IS_LIBRARY="${6}"
readonly SRC_BRANCH DST_BRANCH DEPS KUBERNETES_REMOTE IS_LIBRARY

SCRIPT_DIR=$(dirname "${BASH_SOURCE}")
source "${SCRIPT_DIR}"/util.sh

git checkout "${DST_BRANCH}"
# sync_repo cherry-picks the commits that change
# k8s.io/kubernetes/staging/src/k8s.io/${REPO} to the ${DST_BRANCH}
sync_repo "staging/src/k8s.io/${REPO}" "${SRC_BRANCH}" "${KUBERNETES_REMOTE}"

# update_godeps_json updates the Godeps/Godeps.json to track the latest commits
# of k8s.io/*. 
IFS=',' read -a deps <<< "${DEPS}"
dep_count=${#deps[@]}
for (( i=0; i<${dep_count}; i++ )); do
    update_godeps_json "${deps[i]}"
done

# restore the vendor/ folder. k8s.io/* and github.com/golang/glog will be
# removed from the vendor folder
restore_vendor "${DEPS}" "${IS_LIBRARY}"
