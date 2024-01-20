#!/usr/bin/env bash
# Copyright 2024 The Kubernetes Authors.
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

# generates prowjobs for AR sync

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR=$(dirname "${BASH_SOURCE[0]}")

readonly OUTPUT="${SCRIPT_DIR}/sync-ar-repos.yaml"

readonly AR_REGIONS=(
	asia-east2
	europe-west3
	eureope-west10
	eureope-west12
	us-west3
	us-west4
	southamerica-west1
)

cat >"${OUTPUT}" <<EOF
periodics:
EOF

for ar_region in "${AR_REGIONS[@]}"; do
	cat >>"${OUTPUT}" <<EOF
  - name: sync-to-ar-${ar_region}
    cluster: k8s-infra-prow-build-trusted
    interval: 2h
    decorate: true
    annotations:
      testgrid-dashboards: sig-k8s-infra-registry
      testgrid-tab-name: sync-to-ar-repo-${ar_region}
      testgrid-description: 'Sync AR repo from us-central1 to ${ar_region}'
      testgrid-alert-email: k8s-infra-alerts@kubernetes.io
      testgrid-num-failures-to-alert: '3'
    rerun_auth_config:
      github_team_slugs:
      - org: kubernetes
        slug: sig-k8s-infra-leads
      - org: kubernetes
        slug: release-managers
    spec:
      serviceAccountName: k8s-infra-gcr-promoter
      containers:
      - image: gcr.io/go-containerregistry/gcrane:latest
        imagePullPolicy: Always
        args:
        - cp
        - --recursive
        - --allow-nondistributable-artifacts
        - us-central1-docker.pkg.dev/k8s-artifacts-prod/images
        - ${ar_region}-docker.pkg.dev/k8s-artifacts-prod/images
EOF
done
