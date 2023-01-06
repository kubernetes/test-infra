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

BOOTSTRAP_FETCH_TEST_INFRA=${BOOTSTRAP_FETCH_TEST_INFRA:-"true"}

# clone-or-fetch latest test-infra repo
if ! [[ -d test-infra/.git ]] || [[ "${BOOTSTRAP_FETCH_TEST_INFRA}" != "true" ]]; then
  echo "cloning https://github.com/kubernetes/test-infra"
  rm -rf test-infra
  git clone https://github.com/kubernetes/test-infra
else
  echo "fetching https://github.com/kubernetes/test-infra"
  pushd test-infra >/dev/null
  remote=origin
  git remote set-head ${remote} -a
  # auto-detect default remote branch name
  branch=$(<".git/refs/remotes/${remote}/HEAD" sed -e "s|ref: refs/remotes/${remote}/||")
  git fetch "${remote}" "${branch}" && git reset --hard "${remote}/${branch}"
  # but don't bother setting local branch name yet; scenario-based jobs still assume 'master'
  popd >/dev/null
fi

BOOTSTRAP_UPLOAD_BUCKET_PATH=${BOOTSTRAP_UPLOAD_BUCKET_PATH:-"gs://kubernetes-jenkins/logs"}

# actually start bootstrap and the job, under the runner (which handles dind etc.)
/usr/local/bin/runner.sh \
    ./test-infra/jenkins/bootstrap.py \
        --job="${JOB_NAME}" \
        --service-account="${GOOGLE_APPLICATION_CREDENTIALS}" \
        --upload="${BOOTSTRAP_UPLOAD_BUCKET_PATH}" \
        "$@"
