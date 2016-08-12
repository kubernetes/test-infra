#!/bin/bash
# Copyright 2016 The Kubernetes Authors All rights reserved.
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
    echo "usage: publisher source_dir destination_dir source_url destination_url token temp_mem_dir. source_dir, destination_dir and temp_mem_dir are expected to be absolute paths."
    exit 1
fi
SRC="${1}"
DST="${2}"
SRCURL="${3}"
DSTURL="${4}"
TOKEN="${5}"
NETRCDIR="${6}"
# set up github token
echo "machine github.com login ${TOKEN}" > "${NETRCDIR}"/.netrc
rm -f ~/.netrc
ln -s "${NETRCDIR}"/.netrc ~/.netrc
# set up github user
git config --global user.email "k8s-publish-robot@users.noreply.github.com"
git config --global user.name "Kubernetes Publisher"
# get the latest commit hash of source
pushd "${SRC}" > /dev/null
commit_hash=$(git rev-parse HEAD)
popd > /dev/null
# set up the destination directory
rm -rf "${DST}"
mkdir -p "${DST}"
git clone "${DSTURL}" "${DST}"
pushd "${DST}" > /dev/null
rm -r ./*
cp -a "${SRC}/." "${DST}"
git add --all
# check if there are new contents 
if git diff --cached --exit-code &>/dev/null; then
    echo "nothing has changed!"
    exit 0
fi
git commit -m "published by bot, copied from ${SRCURL}, last commit is ${commit_hash}"
git push origin master
popd > /dev/null
rm -f ~/.netrc
rm -f "${NETRCDIR}"/.netrc
