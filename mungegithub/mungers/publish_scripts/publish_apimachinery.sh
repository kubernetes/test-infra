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
# k8s.io/kubernetes/staging/src/apimachinery to the ${dst_branch} of
# k8s.io/apimachinery.
#
# The script assumes that the working directory is
# $GOPATH/src/k8s.io/apimachinery.
#
# The script is expected to be run by
# k8s.io/test-infra/mungegithub/mungers/publisher.go

set -o errexit
set -o nounset
set -o pipefail

if [ ! $# -eq 3 ]; then
    echo "usage: $0 token src_branch dst_branch"
    exit 1
fi

TOKEN="${1}"
# src branch of k8s.io/kubernetes
SRC_BRANCH="${2:-master}"
# dst branch of k8s.io/apimachinery
DST_BRANCH="${3:-master}"
readonly TOKEN SRC_BRANCH DST_BRANCH

SCRIPT_DIR=$(dirname "${BASH_SOURCE}")

git checkout "${DST_BRANCH}"

source "${SCRIPT_DIR}"/util.sh

set_github_token "${TOKEN}"
trap cleanup_github_token EXIT SIGINT

sync_repo "staging/src/k8s.io/apimachinery" "${SRC_BRANCH}"
# restore the vendor/ folder. k8s.io/* and github.com/golang/glog will be
# removed from the vendor folder
restore_vendor
