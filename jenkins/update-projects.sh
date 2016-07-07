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

# Store list of problematic problems
if [[ -f problems.txt ]]; then
  rm problems.txt
fi
touch problems.txt
# Account test projects need
ACCOUNT='kubekins@kubernetes-jenkins.iam.gserviceaccount.com'
# Path we store jenkins job configs
CONFIGS="$(dirname $0)/job-configs"
# List of projects in our job configs
PROJECTS="$(grep -h -r -o -E 'PROJECT=".+"' "${CONFIGS}" | sort | uniq | cut -d '"' -f 2)"
for project in ${PROJECTS}; do
  # Check if account is present and skip if so
  echo -n "${project}: " >&2
  if gcloud projects get-iam-policy "${project}" | grep "${ACCOUNT}" >/dev/null; then
    echo 'configured' >&2
    continue
  fi
  # Try to add the policy and note on stdout if the add fails
  echo 'updating...' >&2
  gcloud -q projects add-iam-policy-binding \
    "$project" \
    --member="serviceAccount:${ACCOUNT}" \
    --role='roles/editor' \
  && echo "${project} updated" >&2 \
  || echo "${project} not updated" | tee -a problems.txt
done
# Check the number of projects that failed to update
echo "There are $(wc -l problems.txt) projects left to update..."
