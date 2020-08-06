#!/usr/bin/env bash
# This script is used to create a new build cluster for use with oss-prow. The cluster will have a 
# single pd-ssd nodepool that will have autoupgrade and autorepair enabled.
#
# Usage: populate the parameters by setting them below or specifying environment variables then run
# the script and follow the prompts. You'll be prompted to share some credentials and commands
# with the current oncall.
#

set -o errexit
set -o nounset
set -o pipefail

TEAM="${TEAM:-}"
PROJECT="${PROJECT:-oss-prow-build-${TEAM}}"
ZONE="${ZONE:-us-west1-b}"
CLUSTER="${CLUSTER:-${PROJECT}}"

# Only needed for creating cluster
MACHINE="${MACHINE:-n1-highmem-8}"
NODECOUNT="${NODECOUNT:-15}"
DISKSIZE="${DISKSIZE:-100GB}"

# Only needed for creating project
FOLDER_ID="${FOLDER_ID:-0123}"
BILLING_ACCOUNT_ID="${BILLING_ACCOUNT_ID:-0123}"  # Find the billing account ID in the cloud console.

# Specific to Prow instance VV
GCSBUCKET="${GCSBUCKET:-oss-prow}"

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
  gcloud projects add-iam-policy-binding "${PROJECT}" --member="group:mdb.cloud-kubernetes-engprod-oncall@google.com" --role="roles/owner"
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
  echo "  gsutil acl ch -u \"${saFull}:O\" \"gs://${GCSBUCKET}\""
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
  echo "  ./merge_kubeconfig_secret.py --secret-key=oss-config ${outfile}"
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