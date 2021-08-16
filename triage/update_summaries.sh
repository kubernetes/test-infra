#!/usr/bin/env bash

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

# dataset table to query for build info
readonly BUILD_DATASET_TABLE="${BUILD_DATASET_TABLE:-"k8s-gubernator:build.all"}"

# dataset to write temp results to for triage
readonly TRIAGE_DATASET_TABLE="${TRIAGE_DATASET_TABLE:-"k8s-gubernator:temp.triage"}"

# gcs bucket to write temporary results to
readonly TRIAGE_TEMP_GCS_PATH="${TRIAGE_TEMP_GCS_PATH:-"gs://k8s-gubernator/triage_tests"}"

# gcs uri to write final triage results to
readonly TRIAGE_GCS_PATH="${TRIAGE_GCS_PATH:-"gs://k8s-gubernator/triage"}"

# the gcp project against which to bill bq usage
readonly TRIAGE_BQ_USAGE_PROJECT="${TRIAGE_BQ_USAGE_PROJECT:-"k8s-gubernator"}"

cd "$(dirname "$0")"

start=$(date +%s)

if [[ -e ${GOOGLE_APPLICATION_CREDENTIALS-} ]]; then
  echo "activating service account with credentials at: ${GOOGLE_APPLICATION_CREDENTIALS}"
  gcloud auth activate-service-account --key-file="${GOOGLE_APPLICATION_CREDENTIALS}"
fi

gcloud config set project "${TRIAGE_BQ_USAGE_PROJECT}"

bq show <<< $'\n'

date

# populate triage_builds.json with build metadata
bq --project_id="${TRIAGE_BQ_USAGE_PROJECT}" --headless --format=json query --max_rows 1000000 \
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
    [${BUILD_DATASET_TABLE}]
  where
    timestamp_to_sec(started) > TIMESTAMP_TO_SEC(DATE_ADD(CURRENT_DATE(), -14, 'DAY'))
    and job != 'ci-kubernetes-coverage-unit'" \
  > triage_builds.json

# populate ${TRIAGE_DATASET_TABLE} with test failures
bq --project_id="${TRIAGE_BQ_USAGE_PROJECT}" query --allow_large_results --headless --max_rows 0 --replace --destination_table "${TRIAGE_DATASET_TABLE}" \
  "select
    timestamp_to_sec(started) started,
    path build,
    test.name name,
    test.failure_text failure_text
  from
    [${BUILD_DATASET_TABLE}]
  where
    test.failed
    and timestamp_to_sec(started) > TIMESTAMP_TO_SEC(DATE_ADD(CURRENT_DATE(), -14, 'DAY'))
    and job != 'ci-kubernetes-coverage-unit'"

gsutil rm "${TRIAGE_TEMP_GCS_PATH}/shard_*.json.gz" || true
bq extract --compression GZIP --destination_format NEWLINE_DELIMITED_JSON "${TRIAGE_DATASET_TABLE}" "${TRIAGE_TEMP_GCS_PATH}/shard_*.json.gz"
mkdir -p triage_tests
gsutil cp -r "${TRIAGE_TEMP_GCS_PATH}/*" triage_tests/
gzip -df triage_tests/*.gz

# gsutil cp "${TRIAGE_BUCKET}/failure_data.json failure_data_previous.json

mkdir -p slices

/triage \
  --builds triage_builds.json \
  --output failure_data.json \
  --output_slices slices/failure_data_PREFIX.json \
  ${NUM_WORKERS:+"--num_workers=${NUM_WORKERS}"} \
  triage_tests/*.json

gsutil_cp() {
  gsutil -h 'Cache-Control: no-store, must-revalidate' -m cp -Z "$@"
}

gsutil_cp failure_data.json "${TRIAGE_GCS_PATH}/"
gsutil_cp slices/*.json "${TRIAGE_GCS_PATH}/slices/"
gsutil_cp failure_data.json "${TRIAGE_GCS_PATH}/history/$(date -u +%Y%m%d).json"

stop=$(date +%s)
elapsed=$(( stop - start ))
echo "Finished in $(( elapsed / 60))m$(( elapsed % 60))s"
