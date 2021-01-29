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

# This script is used to create a new build cluster for use with prow. The cluster will have a 
# single pd-ssd nodepool that will have autoupgrade and autorepair enabled.
#
# Usage: populate the parameters by setting them below or specifying environment variables then run
# the script and follow the prompts. You'll be prompted to share some credentials and commands
# with the current oncall.
#

set -o errexit
set -o nounset
set -o pipefail

# Specific to Prow instance
PROW_INSTANCE_NAME="${PROW_INSTANCE_NAME:-}"
GCS_BUCKET="${GCS_BUCKET:-${PROW_INSTANCE_NAME}}"

# Specific to the build cluster
TEAM="${TEAM:-}"
PROJECT="${PROJECT:-${PROW_INSTANCE_NAME}-build-${TEAM}}"
ZONE="${ZONE:-us-west1-b}"
CLUSTER="${CLUSTER:-${PROJECT}}"

# Only needed for creating cluster
MACHINE="${MACHINE:-n1-standard-8}"
NODECOUNT="${NODECOUNT:-5}"
DISKSIZE="${DISKSIZE:-100GB}"

# Only needed for creating project
FOLDER_ID="${FOLDER_ID:-0123}"
BILLING_ACCOUNT_ID="${BILLING_ACCOUNT_ID:-0123}"  # Find the billing account ID in the cloud console.
ADMIN_IAM_MEMBER="${ADMIN_IAM_MEMBER:-group:mdb.cloud-kubernetes-engprod-oncall@google.com}"

# Overriding output
OUT_FILE="${OUT_FILE:-build-cluster-kubeconfig.yaml}"

function main() {
  parseArgs "$@"
  prompt "Create project" createProject
  prompt "Create cluster" createCluster
  prompt "Create a SA and secret for uploading results to GCS" createUploadSASecret
  prompt "Generate kubeconfig credentials for Prow" gencreds
  echo "All done!"
}
# Prep and check args.
function parseArgs() {
  for var in TEAM PROJECT ZONE CLUSTER MACHINE NODECOUNT DISKSIZE FOLDER_ID BILLING_ACCOUNT_ID; do
    if [[ -z "${!var}" ]]; then
      echo "Must specify ${var} environment variable (or specify a default in the script)."
      exit 2
    fi
    echo "${var}=${!var}"
  done
}
function prompt() {
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
function pause() {
  read -n 1 -s -r
}

authed=""
function getClusterCreds() {
  if [[ -z "${authed}" ]]; then
    gcloud container clusters get-credentials --project="${PROJECT}" --zone="${ZONE}" "${CLUSTER}"
    authed="true"
  fi
}
function createProject() {
  # Create project, configure billing, enable GKE, add IAM rule for oncall team.
  echo "Creating project '${PROJECT}' (this may take a few minutes)..."
  gcloud projects create "${PROJECT}" --name="${PROJECT}" --folder="${FOLDER_ID}"
  gcloud beta billing projects link "${PROJECT}" --billing-account="${BILLING_ACCOUNT_ID}"
  gcloud services enable "container.googleapis.com" --project="${PROJECT}"
  gcloud projects add-iam-policy-binding "${PROJECT}" --member="${ADMIN_IAM_MEMBER}" --role="roles/owner"
}
function createCluster() {
  echo "Creating cluster '${CLUSTER}' (this may take a few minutes)..."
  echo "If this fails due to insufficient project quota, request more at https://console.cloud.google.com/iam-admin/quotas?project=${PROJECT}"
  echo
  gcloud container clusters create "${CLUSTER}" --project="${PROJECT}" --zone="${ZONE}" --machine-type="${MACHINE}" --num-nodes="${NODECOUNT}" --disk-size="${DISKSIZE}" --disk-type="pd-ssd" --enable-autoupgrade --enable-autorepair
  getClusterCreds
  kubectl create namespace "test-pods"
}
function createUploadSASecret() {
  getClusterCreds
  local sa="prow-pod-utils"
  local saFull="${sa}@${PROJECT}.iam.gserviceaccount.com"
  # Create a service account for uploading to GCS.
  gcloud beta iam service-accounts create "${sa}" --project="${PROJECT}" --description="SA for Prow's pod utilities to use to upload job results to GCS." --display-name="Prow Pod Utilities"
  # Generate private key and attach to the service account.
  gcloud iam service-accounts keys create "sa-key.json" --project="${PROJECT}" --iam-account="${saFull}"
  kubectl create secret generic "service-account" -n "test-pods" --from-file="service-account.json=sa-key.json"
  echo
  echo "Please ask the test-infra oncall (https://go.k8s.io/oncall) to run the following:"
  echo "  gsutil acl ch -u \"${saFull}:O\" \"gs://${GCS_BUCKET}\""
  echo
  echo "Press any key to aknowledge (this doesn't need to be completed to continue this script, but it needs to be done before uploading will work)..."
  pause
}

origdir="$( pwd -P )"
tempdir="$( mktemp -d )"
# generate a JWT kubeconfig file that we can merge into prow's kubeconfig secret so that Prow can schedule pods
function gencreds() {
  getClusterCreds
  local clusterAlias="build-${TEAM}"
  local outfile="${OUT_FILE}"
  # TODO: Make gencred build without bazel so we can use something like the following:
  # GO111MODULE=on go run k8s.io/test-infra/gencred --serviceaccount --name "${clusterAlias}"
  cd "${tempdir}"
  git clone https://github.com/kubernetes/test-infra --depth=1
  cd test-infra
  bazel run //gencred -- --context="$(kubectl config current-context)" --name "${clusterAlias}" > "${origdir}/${outfile}"
  cd "${origdir}"
  echo
  echo "Supply the file '${outfile}' to the current oncall for them to add to Prow's kubeconfig secret via:"
  echo "  kubectl --context=<kubeconfig-context-for-prow-cluster> create secret generic kubeconfig-${clusterAlias} --from-file=kubeconfig=${outfile}"
  echo "  Prow oncall should create PR updating build clusters consuming prow components to also include this new build cluster as part of KUBECONFIG env var."
  echo "  An example of this PR is https://github.com/GoogleCloudPlatform/oss-test-infra/pull/653"
  echo "ProwJobs that intend to use this cluster should specify 'cluster: ${clusterAlias}'" # TODO: color this
  echo
  echo "Press any key to acknowledge (this doesn't need to be completed to continue this script, but it needs to be done before Prow can schedule jobs to your cluster)..."
  pause
}
function cleanup() {
  returnCode="$?"
  rm -f "sa-key.json" || true
  rm -rf "${tempdir}" || true
  exit "${returnCode}"
}
trap cleanup EXIT
main "$@"
cleanup