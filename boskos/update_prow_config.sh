#!/usr/bin/env bash
# Copyright 2017 The Kubernetes Authors.
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

# Usage: updade_prow_config.sh

set -o errexit
set -o nounset
set -o pipefail

cd "$(git rev-parse --show-toplevel)"

# Script triggered by prow postsubmit job
# Update boskos configmap in prow

if [[ -a "${PROW_SERVICE_ACCOUNT:-}" ]] ; then # TODO(fejta): delete this in a subsequent PR
  echo "Use GOOGLE_APPLICATION_CREDENTIALS='$PROW_SERVICE_ACCOUNT' " >&2
  echo "Migrate away from PROW_SERVICE_ACCOUNT" >&2
  gcloud auth activate-service-account --key-file="${PROW_SERVICE_ACCOUNT}"
elif [[ -a "${GOOGLE_APPLICATION_CREDENTIALS:-}" ]] ; then
  gcloud auth activate-service-account --key-file="${GOOGLE_APPLICATION_CREDENTIALS}"
fi

if ! [ -x "$(command -v kubectl)" ]; then
  gcloud components install kubectl
fi

# TODO(fejta): deploy this using the bazel target
gcloud container clusters get-credentials --project=k8s-prow-builds --zone=us-central1-f prow
kubectl create configmap resources --from-file=config=config/prow/cluster/boskos-resources.yaml --dry-run -o yaml | \
    kubectl --context=gke_k8s-prow-builds_us-central1-f_prow --namespace=test-pods replace configmap resources -f -
