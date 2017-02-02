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
# k8s.io/kubernetes/staging/src/client-go to the ${dst_branch} of
# k8s.io/client-go.
#
# The script assumes that the working directory is $GOPATH/src/k8s.io/client-go.
# It also assumes $GOPATH/k8s.io/apimachinery has been updated.
#
# The script is expected to be run by
# k8s.io/test-infra/mungegithub/mungers/publisher.go

set -o errexit
set -o nounset
set -o pipefail

if [ ! $# -eq 2 ]; then
    echo "usage: $0 src_branch dst_branch"
    exit 1
fi

# src branch of k8s.io/kubernetes
SRC_BRANCH="${1:-master}"
# dst branch of k8s.io/client-go
DST_BRANCH="${2:-master}"
readonly SRC_BRANCH DST_BRANCH

SCRIPT_DIR=$(dirname "${BASH_SOURCE}")
source "${SCRIPT_DIR}"/util.sh

git checkout "${DST_BRANCH}"
# this currently only updates commit hash of k8s.io/apimachinery
update_godeps_json() {
    local godeps_json="./Godeps/Godeps.json"
    local old_revs=""
    local new_rev=$(cd ../apimachinery; git rev-parse HEAD)

    # TODO: simplify the following lines
    while read path rev; do
        if [[ "${path}" == "k8s.io/apimachinery"* ]]; then
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

basic_tests() {
    go build ./...
    go test ./...
}

# sync with kubernetes/staging, commit the changes
sync_repo "staging/src/k8s.io/client-go" "${SRC_BRANCH}"
# update the Godeps.json. The dummy revision of k8s.io/apimachinery entries will
# be updated with the latest commit hash.
update_godeps_json
# restore the vendor/ folder. k8s.io/* and github.com/golang/glog will be
# removed from the vendor folder
restore_vendor
# Smoke test client-go
basic_tests
