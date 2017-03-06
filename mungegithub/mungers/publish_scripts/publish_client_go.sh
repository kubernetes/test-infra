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

set -o errexit
set -o nounset
set -o pipefail

echo $@
if [ ! $# -eq 6 ]; then
    echo "usage: $0 destination_dir destination_branch token commit_message
    gopath. destination_dir is expected to be absolute paths."
    exit 1
fi
DST="${1}"
DST_BRANCH="${2}"
TOKEN="${3}"
MESSAGE="${4}"
GOPATH="${5}"

# set up github token
mv ~/.netrc ~/.netrc.bak || true
echo "machine github.com login ${TOKEN}" > ~/.netrc

cleanup() {
    rm -rf ~/.netrc
    mv ~/.netrc.bak ~/.netrc || true
}
trap cleanup EXIT SIGINT

pushd "${DST}" > /dev/null
git add --all
# check if there are new contents 
if git diff --cached --exit-code &>/dev/null; then
    echo "nothing has changed!"
    exit 0
fi
git -c user.name="Kubernetes Publisher" -c user.email="k8s-publish-robot@users.noreply.github.com" commit -m "${MESSAGE}"

# Run "godep restore" to restore dependencies. Because entries for
# k8s.io/apimachinery are removed from Godeps.json, so the just published code
# for apimachinery will not be overwritten. Then we run "godep save" to save all
# the dependencies, including the latest commit of k8s.io/apimachinery.
godep restore
godep save ./...
git add --all
if git diff --cached --exit-code &>/dev/null; then
    echo "dependency has not changed!"
else
    git -c user.name="Kubernetes Publisher" -c user.email="k8s-publish-robot@users.noreply.github.com" commit -m "pick up new dependencies on k8s.io repos"
fi

go build ./...
go test ./...

git push origin "${DST_BRANCH}"
popd > /dev/null
