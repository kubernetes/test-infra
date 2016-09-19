#!/bin/bash

# Copyright 2016 The Kubernetes Authors.
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

# Save environment variables in $WORKSPACE/env.list and then run the Jenkins e2e
# test runner inside the kubekins-test Docker image.

set -o errexit
set -o nounset
set -o pipefail

export REPO_DIR=${REPO_DIR:-$(pwd)}
export HOST_ARTIFACTS_DIR=${WORKSPACE}/_artifacts
mkdir -p "${HOST_ARTIFACTS_DIR}"

# TODO(ixdy): remove when all jobs are setting these vars using Jenkins credentials
: ${JENKINS_GCE_SSH_PRIVATE_KEY_FILE:='/var/lib/jenkins/gce_keys/google_compute_engine'}
: ${JENKINS_GCE_SSH_PUBLIC_KEY_FILE:='/var/lib/jenkins/gce_keys/google_compute_engine.pub'}

KUBEKINS_E2E_IMAGE_TAG='v20160916-b08fd17'
KUBEKINS_E2E_IMAGE_TAG_OVERRIDE_FILE="${WORKSPACE}/hack/jenkins/.kubekins_e2e_image_tag"
if [[ -r "${KUBEKINS_E2E_IMAGE_TAG_OVERRIDE_FILE}" ]]; then
  KUBEKINS_E2E_IMAGE_TAG=$(cat "${KUBEKINS_E2E_IMAGE_TAG_OVERRIDE_FILE}")
fi

env \
  -u GOOGLE_APPLICATION_CREDENTIALS \
  -u GOROOT \
  -u HOME \
  -u PATH \
  -u PWD \
  -u WORKSPACE \
  >${WORKSPACE}/env.list

docker_extra_args=()
if [[ "${JENKINS_ENABLE_DOCKER_IN_DOCKER:-}" =~ ^[yY]$ ]]; then
    docker_extra_args+=(\
      -v /var/run/docker.sock:/var/run/docker.sock \
      -v "${REPO_DIR}":/go/src/k8s.io/kubernetes \
      -e "REPO_DIR=${REPO_DIR}" \
      -e "HOST_ARTIFACTS_DIR=${HOST_ARTIFACTS_DIR}" \
    )
fi

if [[ "${KUBE_RUN_FROM_OUTPUT:-}" =~ ^[yY]$ ]]; then
    docker_extra_args+=(\
      -v "${WORKSPACE}/_output":/workspace/_output:ro \
    )
fi

# Timeouts can leak the container, causing weird issues where the job keeps
# running and *deletes resources* that the next job is using.
# Give the container a unique name, and stop it when bash EXITs,
# which will happen during a timeout.
CONTAINER_NAME="${JOB_NAME}-${BUILD_NUMBER}"
trap "docker stop ${CONTAINER_NAME}" EXIT

echo "Starting..."
docker run --rm -i \
  --name="${CONTAINER_NAME}" \
  -v "${WORKSPACE}/_artifacts":/workspace/_artifacts \
  -v /etc/localtime:/etc/localtime:ro \
  ${JENKINS_GCE_SSH_PRIVATE_KEY_FILE:+-v "${JENKINS_GCE_SSH_PRIVATE_KEY_FILE}:/workspace/.ssh/google_compute_engine:ro"} \
  ${JENKINS_GCE_SSH_PUBLIC_KEY_FILE:+-v "${JENKINS_GCE_SSH_PUBLIC_KEY_FILE}:/workspace/.ssh/google_compute_engine.pub:ro"} \
  ${JENKINS_AWS_SSH_PRIVATE_KEY_FILE:+-v "${JENKINS_AWS_SSH_PRIVATE_KEY_FILE}:/workspace/.ssh/kube_aws_rsa:ro"} \
  ${JENKINS_AWS_SSH_PUBLIC_KEY_FILE:+-v "${JENKINS_AWS_SSH_PUBLIC_KEY_FILE}:/workspace/.ssh/kube_aws_rsa.pub:ro"} \
  ${JENKINS_AWS_CREDENTIALS_FILE:+-v "${JENKINS_AWS_CREDENTIALS_FILE}:/workspace/.aws/credentials:ro"} \
  ${GOOGLE_APPLICATION_CREDENTIALS:+-v "${GOOGLE_APPLICATION_CREDENTIALS}:/service-account.json:ro"} \
  --env-file "${WORKSPACE}/env.list" \
  -e "HOME=/workspace" \
  -e "WORKSPACE=/workspace" \
  ${GOOGLE_APPLICATION_CREDENTIALS:+-e "GOOGLE_APPLICATION_CREDENTIALS=/service-account.json"} \
  "${docker_extra_args[@]:+${docker_extra_args[@]}}" \
  "gcr.io/google-containers/kubekins-e2e:${KUBEKINS_E2E_IMAGE_TAG}" && rc=$? || rc=$?

trap '' EXIT  # container exited
exit ${rc}
