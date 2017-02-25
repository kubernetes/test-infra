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

# This script publishes the latest changes in the master branch of
# k8s.io/kubernetes/staging/src/apimachinery to the master branch of
# k8s.io/apimachinery.
#
# The script assumes that the working directory is
# $GOPATH/src/k8s.io/apimachinery/, the master branch is checked out and is
# tracking the master branch of https://github.com/kubernetes/apimachinery.
#
# The script is expected to be run by
# k8s.io/test-infra/mungegithub/mungers/publisher.go

if [ ! $# -eq 4 ]; then
    echo "usage: $0 token src_branch dst_branch"
    exit 1
fi

TOKEN="${1}"
SRC_BRANCH="${2}"
DST_BRANCH="${3}"
readonly TOKEN SRC_BRANCH DST_BRANCH

source ./util.sh

# set up github token
mv ~/.netrc ~/.netrc.bak || true
echo "machine github.com login ${TOKEN}" > ~/.netrc

cleanup() {
    rm -rf ~/.netrc
    mv ~/.netrc.bak ~/.netrc || true
}
trap cleanup EXIT SIGINT

# this currently only updates commit hash of k8s.io/apimachinery
update_godeps_json() {
    godeps_json="./Godeps/Godeps.json"
    dir=$(mktemp -d "${TMPDIR:-/tmp/}$(basename $0).XXXXXXXXXXXX")
    new_rev=$(cd ../apimachinery; git rev-parse HEAD)
    while read path rev; do
        if [[ "${path}" == "k8s.io/apimachinery"* ]]; then
            echo "${rev}" > "${dir}"/rev
        fi
    done < <(jq '.Deps|.[]|.ImportPath + " " + .Rev' -r < "${godeps_json}")
    cat "${dir}"/rev | uniq > "${dir}"/rev
    while read oldrev; do
        sed -i 's|"${oldrev}"|"${newrev}"|g' "${godeps_json}"
    done < <(cat "${dir}"/rev)
}

basic_tests() {
    go build ./...
    go test ./...
}

# sync with kubernetes/staging, commit the changes
sync "staging/src/k8s.io/client-go" "{SRC_BRANCH}"
# update the Godeps.json. The dummy revision of k8s.io/apimachinery entries will
# be updated with the latest commit hash.
update_godeps_json
# restore the vendor/ folder. k8s.io/* and github.com/golang/glog will be
# removed from the vendor folder
restore_vendor
# Smoke test client-go
basic_tests

# publish
git push origin $(git rev-parse --abbrev-ref HEAD)
