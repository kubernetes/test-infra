#!/bin/bash
# Copyright 2019 The Kubernetes Authors.
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

# This script will create a 'prow-deployer' GCP service account with permissions
# to deploy to the GKE cluster and load a service account key into the cluster's
# test-pods namespace. This should only be done when the Prow instance is using a
# separate build cluster and only trusted jobs are running in the service cluster.
# Setting up a deployer service account is necessary for Prow to update itself with
# a postsubmit job.

# To use, point your kubeconfig at the correct cluster context and specify gcp
# PROJECT and service account DESCRIPTION environment variables. Optionally, one can
# supply the PROJECT_BUILD variable to attach the iam policy to the build cluster project.

# To enable prompts and run in "interactive" mode supply the "-i|--interactive" flag.
# e.g.
#  PROJECT="istio-testing" \
#  PROJECT_BUILD="istio-prow-build" \
#  DESCRIPTION="Used to deploy to the clusters in the istio-testing and istio-prow-build projects." \
#  gcloud-deployer-service-account.sh --interactive

# Globals:
PROJECT_BUILD="${PROJECT_BUILD:=}"
SERVICE_ACCOUNT="${SERVICE_ACCOUNT:=prow-deployer}"
# PROJECT => "required"
# DESCRIPTION => "required"

# Options:
INTERACTIVE=

function cleanup() {
  # For security reasons, delete private key regardless of exit code.
  trap 'rm -f "$SERVICE_ACCOUNT-sa-key.json"' EXIT
}

function create_service_account() {
  prompt "Create service-account: \"$SERVICE_ACCOUNT\" in Project: \"$PROJECT\""

  # Create a service account for performing Prow deployments in a GCP project.
  gcloud beta iam service-accounts create $SERVICE_ACCOUNT --project="$PROJECT" --description="$DESCRIPTION" --display-name="Prow Self Deployer SA"

  # Add the `roles/container.admin` IAM policy binding to the service account in "service" cluster project.
  # https://cloud.google.com/kubernetes-engine/docs/how-to/iam#container.admin
  gcloud projects add-iam-policy-binding "$PROJECT" --member="serviceAccount:$SERVICE_ACCOUNT@$PROJECT.iam.gserviceaccount.com" --role "roles/container.admin"

  # Generate private key and attach to the service account.
  gcloud iam service-accounts keys create "$SERVICE_ACCOUNT-sa-key.json" --project="$PROJECT" --iam-account="$SERVICE_ACCOUNT@$PROJECT.iam.gserviceaccount.com"

  if [ "$PROJECT_BUILD" ]; then
    prompt "Apply iam policy to build Project: \"$PROJECT_BUILD\""

    # Add the `roles/container.admin` IAM policy binding to the service account in "build" cluster project.
    gcloud projects add-iam-policy-binding "$PROJECT_BUILD" --member="serviceAccount:$SERVICE_ACCOUNT@$PROJECT.iam.gserviceaccount.com" --role "roles/container.admin"
  fi
}

function create_secret() {
  prompt "Create cluster secret for Kube context: \"$(kubectl config current-context)\""

  # Deploy the service-account secret to the cluster in the current context.
  kubectl create secret generic -n test-pods "$SERVICE_ACCOUNT-service-account" --from-file="service-account.json=$SERVICE_ACCOUNT-sa-key.json"
}

function handle_options() {
  while [ $# -gt 0 ]; do
    case "$1" in
    -i | --interactive)
      INTERACTIVE=1
      shift
      ;;
    *)
      echo "Unknown option: $1" >&1
      exit 1
      ;;
    esac
  done
}

function prompt() {
  if [ "$INTERACTIVE" ]; then
    echo
    read -r -n1 -p "$1 ? [y/n] "
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
      exit 0
    fi
  fi
}

function main() {
  cleanup
  handle_options "$@"
  create_service_account
  create_secret
}

main "$@"
