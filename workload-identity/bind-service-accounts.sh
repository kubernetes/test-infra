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


set -o errexit
set -o nounset
set -o pipefail

if [[ $# != 6 ]]; then
  echo "Usage: $(basename "$0") <project> <zone> <cluster> <namespace> <name> <gcp-service-account>" >&2
  exit 1
fi

project=$1
zone=$2
cluster=$3
context="gke_${project}_${zone}_${cluster}"
namespace=$4
name=$5
gcp_service_account=$6

current-annotation() {
  kubectl get serviceaccounts \
    "--context=$context" "--namespace=$namespace" "$name" \
    -o jsonpath="{.metadata.annotations.iam\.gke\.io/gcp-service-account}"
}

current=$(current-annotation || echo MISSING)

if [[ "$current" != "$gcp_service_account" ]]; then
  echo "Service account has wrong/missing annotation, please declare the following to $namespace/$name in $context:" >&2
  echo '"{"metadata": {"annotations": "iam.gke.io/gcp-service-account": '"\"$gcp_service_account\"}}"
  exit 1
fi

# Extract GOAL from someone@GOAL.iam.gserviceaccount.com
gcp_sa_project=${gcp_service_account##*@}
gcp_sa_project=${gcp_sa_project%%.*}

role=roles/iam.workloadIdentityUser
members=($(
  gcloud iam service-accounts get-iam-policy \
    "--project=$gcp_sa_project" "$gcp_service_account" \
    --filter="bindings.role=$role" \
    --flatten=bindings --format='value[delimiter=" "](bindings.members)'
))

want="serviceAccount:$project.svc.id.goog[$namespace/$name]"
fix_policy=yes
for member in "${members[@]}"; do
  if [[ "$want" == "$member" ]]; then
    fix_policy=
    break
  fi
done


if [[ -z "${fix_policy}" ]]; then
    echo "ALREADY MEMBER: $want has $role for $gcp_service_account."
else
  (
    set -o xtrace
    gcloud iam service-accounts add-iam-policy-binding \
      "--project=$gcp_sa_project" \
      --role=roles/iam.workloadIdentityUser \
      "--member=$want" \
      $gcp_service_account
  ) > /dev/null
  echo "Sleeping 2m to allow credentials to propagate.." >&2
  sleep 2m
fi

pod-identity() {
  head -n 1 <(
    entropy=$(date +%S)
    set -o xtrace
    kubectl run --rm=true -i --generator=run-pod/v1 \
      "--context=$context" "--namespace=$namespace" "--serviceaccount=$name" \
      --image=google/cloud-sdk:slim "workload-identity-test-$entropy" \
      <<< "gcloud config get-value core/account"
  )
}

# Filter out the  the "try pressing enter" message from stderr
got=$((pod-identity 3>&1 1>&2 2>&3 | grep -v "try pressing enter") 3>&1 1>&2 2>&3)
if [[ "$got" != "$gcp_service_account" ]]; then
  echo "Bad identity, got $got, want $gcp_service_account" >&2
  exit 1
fi

echo "DONE: --context=$context --namespace=$namespace serviceaccounts/$name acts as $gcp_service_account"
