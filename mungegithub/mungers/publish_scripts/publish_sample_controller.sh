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
# k8s.io/kubernetes/staging/src/sample-apiserver to the ${dst_branch} of
# k8s.io/sample-apiserver.
#
# ${kubernetes_remote} is the remote url of k8s.io/kubernetes that will be used
# in .git/config in the local checkout of sample-apiserver. We usually set it to
# the local checkout of k8s.io/kubernetes to avoid multiple checkouts.This not
# only reduces the run time, but also ensures all published repos are generated
# from the same revision of k8s.io/kubernetes.
#
# The script assumes that the working directory is
# $GOPATH/src/k8s.io/sample-apiserver.
#
# The script is expected to be run by
# k8s.io/test-infra/mungegithub/mungers/publisher.go

set -o errexit
set -o nounset
set -o pipefail

if [ ! $# -eq 4 ]; then
    echo "usage: $0 src_branch dst_branch dependent_k8s_repos kubernetes_remote"
    exit 1
fi

SCRIPT_DIR=$(dirname "${BASH_SOURCE}")
"${SCRIPT_DIR}"/publish_template.sh "sample-controller" "${1}" "${2}" "${3}" "${4}" "false"
