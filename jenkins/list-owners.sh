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

# Account test projects need
ACCOUNT='kubekins@kubernetes-jenkins.iam.gserviceaccount.com'
# Path we store jenkins job configs
CONFIGS="$(dirname $0)/job-configs"
# List of projects in our job configs
PROJECTS="$(grep -h -r -o -E 'PROJECT=".+"' "${CONFIGS}" | sort | uniq | cut -d '"' -f 2)"

(for project in ${PROJECTS}; do
  # Check each project to see if it has the service account
  echo -n "${project}: " >&2
  gcloud projects get-iam-policy "${project}" | grep "${ACCOUNT}" >/dev/null \
  && echo 'configured' >&2 \
  && continue  # We are done if it is there
  # Otherwise list owners of the account
  echo 'listing owners...' >&2
  gcloud projects get-iam-policy "${project}" \
    --format='json(bindings)' | jq -r ".bindings[] | select(.role == \"roles/owner\") | .members[] "
done) | sort | uniq -c | sort -h
