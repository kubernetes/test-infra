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

set -o xtrace
set -o errexit

NAME='queue-health'
PROJECT='kubernetes-jenkins'
STAGE_PATH="gs://kubernetes-test-history/sq/"

echo "Copy files to ${STAGE_PATH}..."
gsutil -m cp * "${STAGE_PATH}"

echo "Check for previous version of ${NAME}..."
PREVIOUS="$(gcloud compute instances list \
  --filter="name=${NAME}" --project="${PROJECT}" || true)"
if [[ -n "${PREVIOUS}" ]]; then
  if [[ "${1}" == "--reset" ]]; then
    echo "Reset ${NAME}..."
    gcloud compute instances reset "${NAME}" --project="${PROJECT}"
    exit 0
  fi
  echo "Delete previous version of ${NAME}..."
  gcloud -q compute instances delete "${NAME}" --project="${PROJECT}"
fi

echo "Create ${NAME}..."
gcloud compute instances create \
  "${NAME}" \
  --boot-disk-size=50GB \
  --description="Created by ${USER} to monitor submit-queue health on $(date)" \
  --machine-type=g1-small \
  --metadata=startup-script-url=gs://kubernetes-test-history/sq/startup.sh \
  --project="${PROJECT}" \
  --scopes=storage-rw
