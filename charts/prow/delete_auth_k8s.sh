#!/bin/sh

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

# Usage:
# The script deletes auth token secrets in the k8s cluster and on github.
#
# Examples:
# ./delete_auth_k8s.sh
#

#set errexit, nounset, pipefail
set -o errexit
set -o nounset
set -o pipefail

GITHUB_USER_NAME=`kubectl get secret hookmanager-cred --output=jsonpath={.data.user_id} | base64 --decode | tr -d '\n\r'`
GITHUB_AUTH_ID=`kubectl get secret hookmanager-cred --output=jsonpath={.data.auth_id} | base64 --decode | tr -d '\n\r'`

#delete the token on github
docker run -it jfelten/hook_manager /hook_manager delete_authorization --account=${GITHUB_USER_NAME} --auth_id=${GITHUB_AUTH_ID}

#delete the token and secret used by this cluster
kubectl delete secret hookmanager-cred