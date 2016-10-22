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
set -o xtrace

if [[ "${HOSTNAME}" == 'jenkins-master' || "${HOSTNAME}" == 'pull-jenkins-master' ]]; then
  sudo gcloud components update
  sudo gcloud components update beta
  sudo gcloud components update alpha
else
  sudo apt-get update || (sudo rm /var/lib/apt/lists/partial/* && sudo apt-get update)
  sudo apt-get install -y google-cloud-sdk
fi
