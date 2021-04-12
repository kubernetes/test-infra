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

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"

readonly PROW_SECRET_ACCESSOR_SA="kubernetes-external-secrets-sa@k8s-prow.iam.gserviceaccount.com"
readonly APPS_CONSUME_KUBECONFIG="crier deck hook pipeline prow_controller_manager sinker"

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

# Macos specific settings
SED="sed"
if command -v gsed &>/dev/null; then
  SED="gsed"
fi
if ! (${SED} --version 2>&1 | grep -q GNU); then
  # darwin is great (not)
  echo "!!! GNU sed is required.  If on OS X, use 'brew install gnu-sed'." >&2
  return 1
fi

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
  echo "  gsutil iam ch \"serviceAccount:${saFull}:roles/storage.objectAdmin\" \"${GCS_BUCKET}\""
  echo
  echo "Press any key to aknowledge (this doesn't need to be completed to continue this script, but it needs to be done before uploading will work)..."
  pause
}

origdir="$( pwd -P )"
tempdir="$( mktemp -d )"
echo
echo "Temporary files produced are stored at: ${tempdir}"
echo

# generate a JWT kubeconfig file that we can merge into prow's kubeconfig secret so that Prow can schedule pods
function gencreds() {
  getClusterCreds
  local clusterAlias="build-${TEAM}"
  local outfile="${OUT_FILE}"

  cd "${tempdir}"
  git clone https://github.com/kubernetes/test-infra --depth=1
  cd test-infra
  go run ./gencred --context="$(kubectl config current-context)" --name="${clusterAlias}" --output="${origdir}/${outfile}" || (
    echo "gencred failed:" >&2
    cat "$origdir/$outfile" >&2
    return 1
  )

  # Store kubeconfig secret in the same project where build cluster is located
  # and set up externalsecret in prow service cluster so that it's synced.
  # First enable secretmanager API, no op if already enabled
  gcloud services enable secretmanager.googleapis.com --project="${PROJECT}"
  cd "${origdir}"
  local gsm_secret_name="prow_build_cluster_kubeconfig_${clusterAlias}"
  gcloud secrets create "${gsm_secret_name}" --data-file="$origdir/$outfile" --project="${PROJECT}"
  # Grant prow service account access to secretmanager in build cluster
  for role in roles/secretmanager.viewer roles/secretmanager.secretAccessor; do
    gcloud beta secrets add-iam-policy-binding "${gsm_secret_name}" --member="serviceAccount:${PROW_SECRET_ACCESSOR_SA}" --role="${role}" --project="${PROJECT}"
  done

  prompt "Create CL for you" create_cl

  echo "ProwJobs that intend to use this cluster should specify 'cluster: ${clusterAlias}'" # TODO: color this
  echo
  echo "Press any key to acknowledge (this doesn't need to be completed to continue this script, but it needs to be done before Prow can schedule jobs to your cluster)..."
  pause
}


ensure_kustomize() {
  if ! which "kustomize" > /dev/null 2>&1; then
    GOBIN=$(pwd)/ GO111MODULE=on go get sigs.k8s.io/kustomize/kustomize/v3
  fi
}

create_cl() {
  cd "${ROOT_DIR}"
  clone_uri="$(git config --get remote.origin.url)"
  fork="$(echo "${clone_uri}" | gsed -e "s;https://github.com/;;" -e "s;git@github.com:;;" -e "s;.git;;")"
  if [[ "${fork}" == "kubernetes/test-infra" ]]; then # This is not a fork
    echo
    read -r -n1 -p "Please provide the clone url for your fork: "
    echo
    clone_uri="$REPLY"
    fork="$(echo "${clone_uri}" | gsed -e "s;https://github.com/;;" -e "s;git@github.com:;;" -e "s;.git;;")"
  fi

  local prow_deployment_dir="./config/prow/cluster"
  cd "${tempdir}"
  ensure_kustomize
  git clone "${clone_uri}" --depth=1 forked-test-infra
  cd forked-test-infra

  git checkout -b add-build-cluster-secret

  cat>>"${prow_deployment_dir}/kubernetes_external_secrets.yaml" <<EOF
---
apiVersion: kubernetes-client.io/v1
kind: ExternalSecret
metadata:
  name: kubeconfig-build-${TEAM}
  namespace: default
spec:
  backendType: gcpSecretsManager
  projectId: ${PROJECT}
  data:
  - key: ${gsm_secret_name}
    name: kubeconfig
    version: latest
EOF

  git add "${prow_deployment_dir}/kubernetes_external_secrets.yaml"
  git commit -m "Add external secret from build cluster for ${TEAM}"
  git push origin HEAD

  git checkout -b use-build-cluster master
  
  for app in ${APPS_CONSUME_KUBECONFIG}; do
    "${SED}" -i "s;volumeMounts:;volumeMounts:\\
        - mountPath: /etc/${TEAM}\\
          name: build-${TEAM}\\
          readOnly: true;" "${prow_deployment_dir}/${app}_deployment.yaml"

    "${SED}" -i "s;volumes:;volumes:\\
      - name: build-${TEAM}\\
        secret:\\
          defaultMode: 420\\
          secretName: kubeconfig-build-${TEAM};" "${prow_deployment_dir}/${app}_deployment.yaml"

    # Appends to an existing value doesn't seem to be supported by kustomize, so
    # using sed instead. `&` represents for regex matched part
    "${SED}" -E -i "s;/etc/kubeconfig/config-[0-9]+;&:/etc/build-${TEAM}/kubeconfig;" "${prow_deployment_dir}/${app}_deployment.yaml"
    git add "${prow_deployment_dir}/${app}_deployment.yaml"
  done

  git commit -m "Add build cluster kubeconfig for ${TEAM}

Please submit this change after the previous PR was submitted and postsubmit job succeeded.
Prow oncall: please don't submit this change until the secret is created successfully, which will be indicated by prow alerts in 2 minutes after the postsubmit job.
"

  git push origin HEAD
  echo
  echo "Please open https://github.com/${fork}/pull/new/add-build-cluster-secret and https://github.com/${fork}/pull/new/use-build-cluster, creating PRs from both of them and assign to test-infra oncall for approval"
  echo
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