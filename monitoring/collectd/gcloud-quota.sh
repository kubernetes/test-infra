#!/bin/bash
# Copyright 2016 The Kubernetes Authors All rights reserved.
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

# Collects Google Compute quota usage and limits and reports them to Stackdriver.
# Paths have been hard-coded for Google's Jenkins setup, but they can be
# adjusted as necessary.
# See comments in gcloud-quota on how to enable.

set -o errexit
set -o nounset

HOSTNAME="${COLLECTD_HOSTNAME:-localhost}"
INTERVAL="${COLLECTD_INTERVAL:-60}"
PLUGIN='gcloud_quota'
GCLOUD=/usr/local/bin/gcloud
HOME=/var/lib/jenkins

while true; do
    PROJECT=${PROJECT:-$("${GCLOUD}" compute project-info describe --format='value(name)')}
    PREFIX="${HOSTNAME}/${PLUGIN}-${PROJECT}"

    "${GCLOUD}" compute project-info describe --project="${PROJECT}" --format=json | jq -r ".quotas[] | @text \"\
PUTVAL ${PREFIX}/gauge-\(.metric)_usage interval=${INTERVAL} N:\(.usage)
PUTVAL ${PREFIX}/gauge-\(.metric)_limit interval=${INTERVAL} N:\(.limit)
PUTVAL ${PREFIX}/percent-\(.metric) interval=${INTERVAL} N:\(.usage/.limit*100.0)\""

    "${GCLOUD}" compute regions list --project="${PROJECT}" --format=json | jq -r ".[] | {region: .name, quota: .quotas[]} | @text \"\
PUTVAL ${PREFIX}/gauge-\(.quota.metric)-\(.region)_usage interval=${INTERVAL} N:\(.quota.usage)
PUTVAL ${PREFIX}/gauge-\(.quota.metric)-\(.region)_limit interval=${INTERVAL} N:\(.quota.limit)
PUTVAL ${PREFIX}/percent-\(.quota.metric)-\(.region) interval=${INTERVAL} N:\(.quota.usage/.quota.limit*100.0)\""
    sleep "${INTERVAL}"
done
