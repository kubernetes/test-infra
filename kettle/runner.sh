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

# A wrapper script for running kettle

# Authenticate gcloud
if [[ -n "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]]; then
  gcloud auth activate-service-account --key-file="${GOOGLE_APPLICATION_CREDENTIALS}"
fi

bq show <<< $'\n' > /dev/null  # create initial bq config

while true; do
  # Attempt to update buckets.yaml.
  curl -fsSL --retry 5 -o buckets.yaml.new https://raw.githubusercontent.com/kubernetes/test-infra/master/kettle/buckets.yaml && mv buckets.yaml.new /kettle/buckets.yaml

  /kettle/update.py
done
