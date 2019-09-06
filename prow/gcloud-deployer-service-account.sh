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
# PROJECT and service account DESCRIPTION environment variables.

gcloud beta iam service-accounts create prow-deployer --project="${PROJECT}" --description="${DESCRIPTION}" --display-name="Prow Self Deployer SA"
gcloud projects add-iam-policy-binding "${PROJECT}" --member="serviceAccount:prow-deployer@${PROJECT}.iam.gserviceaccount.com" --role roles/container.developer
gcloud iam service-accounts keys create prow-deployer-sa-key.json --project="${PROJECT}"  --iam-account="prow-deployer@${PROJECT}.iam.gserviceaccount.com"

kubectl create secret generic -n test-pods prow-deployer-service-account --from-file=service-account.json=prow-deployer-sa-key.json

rm prow-deployer-sa-key.json

