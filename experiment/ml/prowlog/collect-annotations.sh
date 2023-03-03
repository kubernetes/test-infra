#!/bin/bash
# Copyright 2022 The Kubernetes Authors.
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

if [[ $# != 3 ]]; then
  echo "Usage: $(basename "$0") <lookback-duration> <project> <append-to-file>" >&2
  echo >&2
  echo "  Scans google cloud logging for 'Saved selected lines' log messages" >&2
  echo "  and writes/appends them to the specified file as a tsv:" >&2
  echo "  <gcs-url> <start-line> <end-line>" >&2
  exit 1
fi

# TODO(fejta): provide a (preferred?) option to gather this info from GCS

fresh=$1
project=$2
location=$3
(
  set -o xtrace
  gcloud logging read "--project=$project" \
    'jsonPayload.msg:"Saved selected lines"' \
    "--freshness=$fresh" \
    --format='value(jsonPayload.artifact,jsonPayload.start,jsonPayload.end)' \
  | tee -a "$location"
)
sort -u -o "$location" "$location"
