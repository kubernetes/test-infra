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

if [ ! $# -eq 2 ]; then
    echo "usage: $0 github_user_name kube_root"
    exit 1
fi

USER="${1}"
KUBE_ROOT="${2}"

REPOS="apimachinery,api,client-go,apiserver,kube-aggregator,sample-apiserver,apiextensions-apiserver"

IFS=',' read -a repos <<< "${REPOS}"
repo_count=${#repos[@]}

echo "=================="
echo "safety check"
echo "=================="
# safety check
for (( i=0; i<${repo_count}; i++ )); do
    if [ ! -d $KUBE_ROOT/../"${repos[i]}" ]; then
        git clone git@github.com:"${USER}/${repos[i]}".git $KUBE_ROOT/../"${repos[i]}"
    fi
    cd $KUBE_ROOT/../"${repos[i]}"
    if ! git config --get remote.upstream.url >/dev/null; then
        git remote add upstream git@github.com:kubernetes/"${repos[i]}".git
    fi
    if [[ $(git config --get remote.origin.url) != *"${USER}"* ]]; then
        echo "origin is not right, expect to contain ${USER}, got $(git config --get remote.origin.url)"
        exit 1
    fi
    cd -
done

echo "=================="
echo " sync masters"
echo "=================="
for (( i=0; i<${repo_count}; i++ )); do
    cd $KUBE_ROOT/../"${repos[i]}"
    echo "repo=${repos[i]}"
    git fetch upstream
    git checkout master
    git reset --hard upstream/master
    git push -f origin master
done

echo "=================="
echo "sync release-1.6"
echo "=================="

REPOS="apimachinery,apiserver,kube-aggregator,sample-apiserver"
IFS=',' read -a repos <<< "${REPOS}"
repo_count=${#repos[@]}
for (( i=0; i<${repo_count}; i++ )); do
    cd $KUBE_ROOT/../"${repos[i]}"
    echo "repo=${repos[i]}"
    git branch -f release-1.6 upstream/release-1.6
    git push -f origin release-1.6
done

echo "=================="
echo "sync release-1.7"
echo "=================="

REPOS="apimachinery"
IFS=',' read -a repos <<< "${REPOS}"
repo_count=${#repos[@]}
for (( i=0; i<${repo_count}; i++ )); do
    cd $KUBE_ROOT/../"${repos[i]}"
    echo "repo=${repos[i]}"
    git branch -f release-1.7 upstream/release-1.7
    git push -f origin release-1.7
done

# client-go follows semver, so its versions are different from kubernetes.
echo "=================="
echo "sync client-go release-2.0,release-3.0 and release-4.0"
echo "=================="

BRANCHES="release-2.0,release-3.0,release-4.0"
IFS=',' read -a branches <<< "${BRANCHES}"
branch_count=${#branches[@]}
for (( i=0; i<${branch_count}; i++ )); do
    cd "$KUBE_ROOT/../client-go"
    echo "repo=client-go"
    git branch -f "${branches[i]}" upstream/"${branches[i]}" 
    git push -f origin "${branches[i]}"
done
