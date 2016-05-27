#!/usr/bin/env bash

# Copyright 2015 The Kubernetes Authors All rights reserved.
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

IMAGE="gcr.io/google_containers/kubekins-job-builder:4"

# If we're on an old image then stop it and remove it.
if docker inspect job-builder &> /dev/null; then
  if [[ $(docker inspect --format='{{ .Config.Image }}' job-builder) != ${IMAGE} ]]; then
    echo "Removing outdated job-builder container"
    docker stop job-builder > /dev/null
    docker rm job-builder > /dev/null
  fi
fi

# If the container doesn't exist then start it.
if ! docker inspect job-builder &> /dev/null; then
  # jenkins_jobs.ini contains administrative credentials for Jenkins.
  # Store it in the workspace of the Jenkins job that calls this script.
  if [[ -e jenkins_jobs.ini ]]; then
    docker run -idt \
      --net host \
      --name job-builder \
      --restart always \
      -v "${WORKSPACE}:/test-infra" \
      "${IMAGE}"
    docker cp jenkins_jobs.ini job-builder:/etc/jenkins_jobs
  else
    echo "jenkins_jobs.ini not found in workspace" >&2
    exit 1
  fi
fi

docker exec job-builder jenkins-jobs update "${config_dir}"
