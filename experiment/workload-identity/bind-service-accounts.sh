#!/bin/bash

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

role=roles/iam.workloadIdentityUser
members=($(
  gcloud iam service-accounts get-iam-policy "$gcp_service_account" "--project=$project" \
    --filter="bindings.role=$role" \
    --format=json --format='value[delimiter=" "](bindings[0].members)'
))

want="serviceAccount:$project.svc.id.goog[$namespace/$name]"
for member in "${members[@]}"; do
  if [[ "$want" == "$member" ]]; then
    echo "$want already a member of $role for $gcp_service_account, nothing to do."
    exit 0
  fi
done

(
  set -o xtrace
  gcloud iam service-accounts add-iam-policy-binding \
    "--project=$project" \
    --role=roles/iam.workloadIdentityUser \
    "--member=$want" \
    $gcp_service_account
) > /dev/null

pod-identity() {
  (
    set -o xtrace
    kubectl run --rm=true -i --generator=run-pod/v1 \
      "--context=$context" "--namespace=$namespace" "--serviceaccount=$name" \
      --image=google/cloud-sdk:slim workload-identity-test \
      <<< "gcloud config get-value core/account"
  ) | head -n 1
}

got=$(pod-identity 2> >(grep -v "try pressing enter"))
if [[ "$got" != "$gcp_service_account" ]]; then
  echo "Bad identity, got $got, want $gcp_service_account" >&2
  exit 1
fi

echo DONE
