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


set -o nounset
set -o errexit
set -o pipefail

split() {
  project=
  zone=
  cluster=${1##*/} # Remove everything before last /
  if [[ "$cluster" == "$1" ]]; then
    return 0
  fi
  pz=${1%/*} # Remove last / and everything after
  project=${pz%%/*} # Everything before the first /
  zone=${pz##*/} # Everything after the /
}

versions() {
  out=()
  read -r -a out < <(
    gcloud container get-server-config "--project=$project" "--zone=$zone" --format='value(validMasterVersions[0],validNodeVersions[0])'
  )
  master=${out[0]}
  node=${out[1]}
}

current-master-version() {
  gcloud container clusters describe "--project=$project" "--zone=$zone" "$cluster" --format='value(currentMasterVersion)'
}

current-pool-version() {
  gcloud container node-pools describe "--cluster=$cluster" "--project=$project" "--zone=$zone" "$1" --format='value(version)'
}

list-pools() {
  gcloud container node-pools list "--cluster=$cluster" "--project=$project" "--zone=$zone" --format='value(name)'
}

do-upgrade() {
  # Run inside a subshell
  (
    set -o xtrace
    gcloud container clusters upgrade "--project=$project" "--zone=$zone" "$cluster" "$@"
  )
}

upgrade() {
  split "$1"
  versions
  if [[ "$master" == "$(current-master-version)" ]]; then
    echo "Master already at version $master" >&2
  else
    do-upgrade --master "--cluster-version=$master"
  fi
  for pool in $(list-pools); do
    if [[ "$pool" == "$(current-pool-version "$pool")" ]]; then
      echo "$pool already at version $node" >&2
    else
      do-upgrade "--node-pool=$pool" "--cluster-version=$node"
    fi
  done
}

if [[ $# -lt 1 ]]; then
  echo "Usage: $(basename "$0") [cluster | project/zone/cluster ...]" >&2
  exit 1
fi

for i in "$@"; do
  upgrade "$i"
done
