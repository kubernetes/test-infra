#!/usr/bin/env bash
# Copyright 2021 The Kubernetes Authors.
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

# This script is used to create a new GCP service account with permissions need by pod utilities to upload job results to GCS.
# ProwJobs can be configured to use this identity by associating the GCP SA with a K8s SA via workload identity, then
# specifying `default_service_account_name: <K8s SA name>` in the decoration config (can be configured broadly with default decoration configs).
# See github.com/kubernetes/test-infra/workload-identity/ for details about using WI.
#
# This script can also be used to grant the necessary permissions to an existing service account.
# Just skip the first step when prompted.

set -o errexit
set -o nounset
set -o pipefail

PROJECT_ID="${PROJECT_ID:-}"            # GCP Project ID for the service account. e.g. "k8s-prow"
BUCKET="${BUCKET:-}"                    # GCS bucket where job results live. e.g. "gs://k8s-prow"
SA_NAME="${SA_NAME:-}"                  # e.g. "prowjob-default-sa"
# Only needed for service account creation.
SA_DISPLAY_NAME="${SA_DISPLAY_NAME:-}"  # e.g. "Default ProwJob SA"
SA_DESCRIPTION="${SA_DESCRIPTION:-}"    # e.g. "Default SA for ProwJobs that upload to the shared job result bucket."

SA="${SA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com"
main() {
  parseArgs

  prompt "Create service account ${SA}" createSA
  prompt "Grant upload permissions for ${BUCKET} to ${SA}" authorizeUpload

  echo "All done!"
}

# Prep and check args.
parseArgs() {
  for var in SA_NAME PROJECT_ID BUCKET; do
    if [[ -z "${!var}" ]]; then
      echo "Must specify ${var} environment variable (or specify a default in the script)."
      exit 2
    fi
    echo "${var}=${!var}"
  done
}

prompt() {
  local msg="$1" cmd="$2"
  echo
  read -r -n1 -p "$msg ? [y/n] "
  echo

  if [[ $REPLY =~ ^[Yy]$ ]]; then
    "$cmd"
  else
    echo "Skipping and continuing to next step..."
  fi
}

createSA() {
  gcloud beta iam service-accounts create \
    ${SA_NAME} \
    --project="${PROJECT_ID}" \
    --description="${SA_DESCRIPTION}" \
    --display-name="${SA_DISPLAY_NAME}"
}

authorizeUpload() {
  gsutil iam ch "serviceAccount:${SA}:roles/storage.objectAdmin" "${BUCKET}"
}

main "$@"
