#!/usr/bin/env bash
# Copyright 2018 The Kubernetes Authors.
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

# get test-infra for latest bootstrap etc
git clone https://github.com/kubernetes/test-infra
cd test-infra
git fetch origin pull/20273/head
git checkout FETCH_HEAD
cd ..

BOOTSTRAP_UPLOAD_BUCKET_PATH=${BOOTSTRAP_UPLOAD_BUCKET_PATH:-"gs://kubernetes-jenkins/logs"}

# actually start bootstrap and the job, under the runner (which handles dind etc.)
/usr/local/bin/runner.sh \
    ./test-infra/jenkins/bootstrap.py \
        --job="${JOB_NAME}" \
        --service-account="${GOOGLE_APPLICATION_CREDENTIALS}" \
        --upload="${BOOTSTRAP_UPLOAD_BUCKET_PATH}" \
        "$@"
