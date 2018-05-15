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


if [[ -z "${1}" ]]; then
  echo "Usage: $(basename "${0}") </path/to/foo-service-account.json>"
  echo '  Visit https://console.developers.google.com/iam-admin/serviceaccounts/project'
  echo '  Create and download the json key for a service account with storage-rw scope'
  exit 1
fi

path="${1}"

if [[ ! -f "${path}" ]]; then
  echo "Account does not exist"
  exit 1
fi

if ! grep private_key_id "${path}"; then
  echo "${path} does not appear to be a valid .json service account file"
  exit 1
fi

echo 'Copying key to secrets/queue-health-service-account'
kubectl create secret generic queue-health-service-account \
  --from-file=service-account.json="${path}"
