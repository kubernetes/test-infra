#!/bin/bash

# Copyright 2020 The Kubernetes Authors.
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

start=$(date +%s)

if [[ -e ${GOOGLE_APPLICATION_CREDENTIALS-} ]]; then
  gcloud auth activate-service-account --key-file="${GOOGLE_APPLICATION_CREDENTIALS}"
fi

gcloud config set project k8s-gubernator
bq show <<< $'\n'

date

bq --headless --format=json query --max_rows 1000000 \
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
    [k8s-gubernator:build.all]
  where
    timestamp_to_sec(started) > TIMESTAMP_TO_SEC(DATE_ADD(CURRENT_DATE(), -14, 'DAY'))
    and job != 'ci-kubernetes-coverage-unit'" \
  > triage_builds.json

bq query --allow_large_results --headless --max_rows 0 --replace --destination_table k8s-gubernator:temp.triage \
  "select
    timestamp_to_sec(started) started,
    path build,
    test.name name,
    test.failure_text failure_text
  from
    [k8s-gubernator:build.all]
  where
    test.failed
    and timestamp_to_sec(started) > TIMESTAMP_TO_SEC(DATE_ADD(CURRENT_DATE(), -14, 'DAY'))
    and job != 'ci-kubernetes-coverage-unit'"
gsutil rm gs://k8s-gubernator/triage_tests/shard_*.json.gz || true
bq extract --compression GZIP --destination_format NEWLINE_DELIMITED_JSON 'k8s-gubernator:temp.triage' gs://k8s-gubernator/triage_tests/shard_*.json.gz
mkdir -p triage_tests
gsutil cp -r gs://k8s-gubernator/triage_tests/* triage_tests/
gzip -df triage_tests/*.gz

# gsutil cp gs://k8s-gubernator/triage/failure_data.json failure_data_previous.json

mkdir -p slices

/triage \
  --builds triage_builds.json \
  --output failure_data.json \
  --output_slices slices/failure_data_PREFIX.json \
  ${NUM_WORKERS:+"--num_workers=${NUM_WORKERS}"} \
  triage_tests/*.json

gsutil_cp() {
  gsutil -h 'Cache-Control: no-store, must-revalidate' -m cp -Z -a public-read "$@"
}

gsutil_cp failure_data.json gs://k8s-gubernator/triage/
gsutil_cp slices/*.json gs://k8s-gubernator/triage/slices/
gsutil_cp failure_data.json "gs://k8s-gubernator/triage/history/$(date -u +%Y%m%d).json"

stop=$(date +%s)
elapsed=$(( ${stop} - ${start} ))
echo "Finished in $(( ${elapsed} / 60))m$(( ${elapsed} % 60))s"
