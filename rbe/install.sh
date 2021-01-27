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

set -o errexit
set -o nounset
set -o pipefail

if [[ $# -lt 6 ]]; then
    echo "Usage: $(basename "$0") <gcp-project-id> <pool-name> <workers:200> <diskgb:600> <machine:n1-standard-2> <bot ...>" >&2
    exit 1
fi

# Note: this currently requires your project to be added to a private list
# Contact fejta on #sig-testing or #prow on kubernetes slack to get on the
# list
# More info: https://cloud.google.com/remote-build-execution/docs/overview

proj=$1
pool=$2
workers=$3
disk=$4
machine=$5
shift 5

users=()
groups=()
bots=(
  "$@"
)

log() {
    (
        set -o xtrace
        "$@"
    )
}

log gcloud services enable remotebuildexecution.googleapis.com  "--project=$proj"

check=(
  gcloud alpha remote-build-execution
  worker-pools describe
  "$pool" "--project=$proj" --instance=default_instance
)

if [[ -z $pool ]]; then
    echo "Existing pools:" >&2
    for i in $(gcloud alpha remote-build-execution worker-pools list \
        "--project=$proj" \
        --instance=default_instance \
        --format='value(name)'); do
      echo "  $(basename "$i")" >&2
    done
    echo "Usage: $0 $1 <pool>" >&2
    exit 1
fi

if ! "${check[@]}" 2>/dev/null; then
  log gcloud alpha remote-build-execution worker-pools create  \
    "$pool" \
    "--project=$proj" \
    --instance=default_instance \
    "--worker-count=$workers" \
    "--disk-size=$disk" \
    "--machine-type=$machine"
else
  log gcloud alpha remote-build-execution worker-pools update  \
    "$pool" \
    "--project=$proj" \
    --instance=default_instance \
    "--worker-count=$workers" \
    "--disk-size=$disk" \
    "--machine-type=$machine"
fi

# https://cloud.google.com/remote-build-execution/docs/modify-worker-pool
echo "Update remote processing power:
  gcloud alpha remote-build-execution worker-pools update \\
    --project='$proj' \\
    --instance=default_instance \\
    --worker-count='$workers' \\
    --disk-size='$disk' \\
    --machine-type='$machine'
"

members=()

for u in "${users[@]}"; do
    members+=("--member=user:$u")
done

for g in "${groups[@]}"; do
    members+=("--member=group:$g")
done

for b in "${bots[@]}"; do
    members+=("--member=serviceAccount:$b")
done

if [[ "${#members[@]}" -gt 0 ]]; then
    log gcloud projects add-iam-policy-binding "$proj" \
        "${members[@]}" \
        --role=roles/remotebuildexecution.artifactCreator >/dev/null
fi

# https://cloud.google.com/remote-build-execution/docs/access-control
echo "Grant access to users and bots:
  gcloud projects add-iam-policy-binding '$proj' \\
    --role=roles/remotebuildexecution.artifactCreator \\
    --member=user:your.email@example.com \\
    --member:serviceAccount:example.bot@your-project.iam.gserviceaccount.com \\
    --member:group:example-google-group@googlegroups.com
"

echo "Configure your bazel environment:"
echo "  $(dirname "$0")/configure.sh"
