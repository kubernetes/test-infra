#!/usr/bin/env bash

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

# This script will:
#   Create a new GCP project
#   Link a billing account
#   Enable GKE
#   Create a default build cluster

# e.g.
#  create-build-cluster.sh "Test Prow Build" --interactive

# TODO(clarketm): upgrade this to a Python script.

set -euo pipefail

######################################################################

SCRIPT_NAME=$(basename "$0")

# Save current kube context
CURRENT_CONTEXT="$(kubectl config current-context)"

PROJECT_NAME=
PROJECT_ID=
INTERACTIVE=

CLUSTER_NAME="prow"
MACHINE_TYPE="n1-standard-1"
NUM_NODES="3"
ZONE="us-west1-a"
POD_NAMESPACE="test-pods"
FOLDER_ID="396521612403"               # google-default
BILLING_ACCOUNT="00E7A7-4928AF-F1C514" # Cloud Kubernetes Development

declare -A IAM_ROLES=(
  ["group:mdb.cloud-kubernetes-engprod-oncall@google.com"]="roles/owner"
)

######################################################################

prompt() {
  local msg="$1" cmd="$2"

  if [ -z "$INTERACTIVE" ]; then
    "$cmd"
    return
  fi

  echo
  read -r -n1 -p "$msg ? [y/n] "
  echo

  if [[ $REPLY =~ ^[Yy]$ ]]; then
    "$cmd"
  fi
}

# GNU getopt compatibility check
check_getopt() {
  if getopt --test &>/dev/null; then
    {
      echo
      echo "$SCRIPT_NAME requires the use of the \"GNU getopt\" command."
      echo
      echo "brew install gnu-getopt"
      echo "brew link --force gnu-getopt"
      exit 1
    } >&2
  fi
}

get_options() {
  check_getopt

  if opt=$(getopt -o i -l machine-type:,num-nodes:,cluster-name:,zone:,pod-namespace:,interactive -n "$SCRIPT_NAME" -- "$@"); then
    eval set -- "$opt"
  else
    {
      echo
      echo "unable to parse options"
      exit 1
    } >&2
  fi

  while [ $# -gt 0 ]; do
    case "$1" in
    --machine-type)
      MACHINE_TYPE="$2"
      shift 2
      ;;
    --num-nodes)
      NUM_NODES="$2"
      shift 2
      ;;
    --cluster-name)
      CLUSTER_NAME="$2"
      shift 2
      ;;
    --zone)
      ZONE="$2"
      shift 2
      ;;
    --pod-namespace)
      POD_NAMESPACE="$2"
      shift 2
      ;;
    -i | --interactive)
      INTERACTIVE=1
      shift
      ;;
    --)
      shift
      get_params "$@"
      break
      ;;
    *)
      {
        echo
        echo "unknown option: $1"
        exit 1
      } >&2
      ;;
    esac
  done
}

get_params() {
  if [ $# -eq 0 ]; then
    {
      echo
      echo "project name required"
      exit 1
    } >&2
  fi

  PROJECT_NAME="$1"
  PROJECT_ID="$(echo "$PROJECT_NAME" | tr '[A-Z]' '[a-z]' | tr ' ' '-')"
}

# Create GCP project.
create_project() {
  gcloud projects create "$PROJECT_ID" --name="$PROJECT_NAME" --folder="$FOLDER_ID"
}

# Link project to billing account.
link_billing() {
  gcloud beta billing projects link "$PROJECT_ID" --billing-account="$BILLING_ACCOUNT"
}

# Enable Kubernetes Engine API.
enable_kube_api() {
  gcloud services enable "container.googleapis.com" --project="$PROJECT_ID"
}

# Add iam policy bindings.
add_iam() {
  for member in "${!IAM_ROLES[@]}"; do
    gcloud projects add-iam-policy-binding "$PROJECT_ID" --member="$member" --role="${IAM_ROLES[$member]}"
  done
}

# Create cluster.
create_cluster() {
  gcloud container clusters create "$CLUSTER_NAME" --project="$PROJECT_ID" --zone="$ZONE" --machine-type="$MACHINE_TYPE" --num-nodes="$NUM_NODES"
}

# Create pod namespace in cluster.
create_namespace() {
  kubectl create namespace "$POD_NAMESPACE"
}

# Restore kube context.
restore_kube_context() {
  kubectl config use-context "$CURRENT_CONTEXT" &>/dev/null
}

# Authenticate to cluster.
get_creds() {
  gcloud container clusters get-credentials "$CLUSTER_NAME" --project="$PROJECT_ID" --zone="$ZONE"
}

main() {
  get_options "$@"

  # gcloud
  prompt "Create project" create_project
  prompt "Link billing account" link_billing
  prompt "Enable Kubernetes API" enable_kube_api
  prompt "Add IAM bindings" add_iam
  prompt "Create cluster" create_cluster

  get_creds

  # kubectl
  prompt "Create namespace" create_namespace
}

trap 'restore_kube_context' EXIT
main "$@"
