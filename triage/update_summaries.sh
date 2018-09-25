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

set -exu
cd $(dirname $0)

if [[ -e ${GOOGLE_APPLICATION_CREDENTIALS-} ]]; then
  gcloud auth activate-service-account --key-file="${GOOGLE_APPLICATION_CREDENTIALS}"
  gcloud config set project k8s-gubernator
  bq show <<< $'\n'
fi

date

table_mtime=$(bq --format=json show 'k8s-gubernator:build.week' | jq -r '(.lastModifiedTime|tonumber)/1000|floor' )
if [[ ! -e triage_builds.json ]] || [ $(stat -c%Y triage_builds.json) -lt ${table_mtime} ]; then
  echo "UPDATING" $table_mtime
  bq --headless --format=json query -n 1000000 \
    "select
      path,
      timestamp_to_sec(started) started,
      elapsed,
      tests_run,
      tests_failed,
      result,
      executor,
      job,
      number
    from
      [k8s-gubernator:build.week]" > triage_builds.json

  bq query --allow_large_results --headless -n0 --replace --destination_table k8s-gubernator:temp.triage \
    "select
      path build,
      test.name name,
      test.failure_text failure_text
    from
      [k8s-gubernator:build.week]
    where
      test.failed
      and timestamp_to_sec(started) > TIMESTAMP_TO_SEC(DATE_ADD(CURRENT_DATE(), -7, 'DAY'))"
  bq extract --compression GZIP --destination_format NEWLINE_DELIMITED_JSON 'k8s-gubernator:temp.triage' gs://k8s-gubernator/triage_tests.json.gz
  gsutil cp gs://k8s-gubernator/triage-latest.json.gz triage_tests.json.gz
  gzip -d triage_tests.json.gz
fi

gsutil cp gs://k8s-gubernator/triage/failure_data.json failure_data_previous.json
curl -sO --retry 6 https://raw.githubusercontent.com/kubernetes/kubernetes/master/test/test_owners.json

mkdir -p slices

pypy summarize.py triage_builds.json triage_tests.json \
  --previous failure_data_previous.json --owners test_owners.json \
  --output failure_data.json --output_slices slices/failure_data_PREFIX.json

gsutil_cp() {
  gsutil -h 'Cache-Control: no-store, must-revalidate' -m cp -Z -a public-read "$@"
}

gsutil_cp failure_data.json gs://k8s-gubernator/triage/
gsutil_cp slices/*.json gs://k8s-gubernator/triage/slices/
gsutil_cp failure_data.json "gs://k8s-gubernator/triage/history/$(date -u +%Y%m%d).json"
