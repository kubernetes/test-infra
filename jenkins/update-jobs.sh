#!/usr/bin/env bash

# Copyright 2015 The Kubernetes Authors.
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

# Update all Jenkins jobs in a folder specified in $1. It can be the union of
# multiple folders separated with a colon, like with the PATH variable.

if [[ $# -eq 1 ]]; then
  config_dir=$1
else
  echo "Usage: $0 <dir>" >&2
  exit 1
fi

IMAGE="k8s.gcr.io/kubekins-job-builder:5"

# jenkins_jobs.ini contains administrative credentials for Jenkins.
# Store it in /jenkins-master-data.
readonly config_path="/jenkins-master-data/jenkins_jobs.ini"
if [[ -e "${config_path}" ]]; then
  docker run \
    --entrypoint jenkins-jobs \
    --rm \
    --net host \
    --name job-builder \
    -v "${WORKSPACE}/test-infra:/test-infra" \
    -v "${config_path}:/etc/jenkins_jobs/jenkins_jobs.ini:ro" \
    "${IMAGE}" \
    update --delete-old "${config_dir}"
else
  echo "${config_path} not found" >&2
  exit 1
fi
