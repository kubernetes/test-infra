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


set -o nounset
set -o errexit
set -o pipefail

# Enables workload identity on a cluster

if [[ $# != 3 ]]; then
  echo "Usage: $(basename "$0") <project> <zone> <cluster>" >&2
  exit 1
fi

project=$1
zone=$2
cluster=$3


cluster_namespace="$project.svc.id.goog"
pool_metadata=GKE_METADATA


call-gcloud() {
  (
    set -o xtrace
    gcloud beta container "$@" "--project=$project" "--zone=$zone"
  )
}


cluster-identity() {
  call-gcloud clusters describe "$cluster" --format='value(workloadIdentityConfig.identityNamespace)'
}

pool-identities() {
  call-gcloud node-pools list "--cluster=$cluster" --format='value(name)' \
    --filter="config.workloadMetadataConfig.mode != $pool_metadata"
}

fix_service=
service=iamcredentials.googleapis.com

if [[ -z "$(gcloud services list "--project=$project" --filter "name:/$service")" ]]; then
  fix_service=yes
fi

fix_cluster=

actual=$(cluster-identity)
if [[ "$actual" != "$cluster_namespace" ]]; then
  fix_cluster=yes
fi

fix_pools=($(pool-identities))

if [[ -z "$fix_service" && -z "$fix_cluster" && ${#fix_pools[@]} == 0 ]]; then
  echo "Nothing to do"
  exit 0
fi

echo "Enable workload identity on:"
if [[ -n "$fix_service" ]]; then
  echo "  project: $project"
fi
if [[ -n "$fix_cluster" ]]; then
  echo "  cluster: $cluster"
fi
for pool in "${fix_pools[@]}"; do
  echo "  pool: $pool"
done

read -p "Proceed [y/N]:" ans
case $ans in
  y*|Y*)
    ;;
  *)
    echo "ABORTING" >&2
    exit 1
    ;;
esac

if [[ -n "$fix_service" ]]; then
  gcloud services enable "--project=$project" "$service"
fi

if [[ -n "$fix_cluster" ]]; then
  call-gcloud clusters update "$cluster" "--workload-pool=$cluster_namespace"
fi

for pool in "${fix_pools[@]}"; do
  call-gcloud node-pools update --cluster="$cluster" "$pool" "--workload-metadata=$pool_metadata"
done

echo "DONE"
