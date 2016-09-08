#!/bin/bash
# Copyright 2016 The Kubernetes Authors.
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
if [ ! $# -eq 4 ]; then
    echo "usage: publish.sh destination_dir token temp_mem_dir commit_message. destination_dir and temp_mem_dir are expected to be absolute paths."
    exit 1
fi
DST="${1}"
TOKEN="${2}"
NETRCDIR="${3}"
MESSAGE="${4}"
# set up github token
echo "machine github.com login ${TOKEN}" > "${NETRCDIR}"/.netrc
rm -f ~/.netrc
ln -s "${NETRCDIR}"/.netrc ~/.netrc
# set up github user
git config --global user.email "k8s-publish-robot@users.noreply.github.com"
git config --global user.name "Kubernetes Publisher"

pushd "${DST}" > /dev/null
git add --all
# check if there are new contents 
if git diff --cached --exit-code &>/dev/null; then
    echo "nothing has changed!"
    exit 0
fi
git commit -m "${MESSAGE}"
git push origin master
popd > /dev/null
rm -f ~/.netrc
rm -f "${NETRCDIR}"/.netrc
