#!/bin/bash

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
pool_metadata=GKE_METADATA_SERVER


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
  call-gcloud node-pools list "--cluster=$cluster" --format='value(name,config.workloadMetadataConfig.nodeMetadata)'
}

fix_cluster=

actual=$(cluster-identity)
if [[ "$actual" != "$cluster_namespace" ]]; then
  fix_cluster=yes
fi

fix_pools=()

for line in "$(IFS=\n pool-identities)"; do
  pool=${line//$'\t'*}
  meta=${line##*$'\t'}
  if [[ "$meta" != "$pool_metadata" ]]; then
    fix_pools+=("$pool")
  fi
done

if [[ -z "$fix_cluster" && ${#fix_pools[@]} == 0 ]]; then
  echo "Nothing to do"
  exit 0
fi

echo "Enable workload identity on:"
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
    exit 1
    ;;
esac

if [[ -n "$fix_cluster" ]]; then
  call-gcloud clusters update "$cluster" "--identity-namespace=$cluster_namespace"
fi

for pool in "${fix_pools[@]}"; do
  call-gcloud node-pools update --cluster="$cluster" "$pool" "--workload-metadata-from-node=$pool_metadata"
done

echo "DONE"
