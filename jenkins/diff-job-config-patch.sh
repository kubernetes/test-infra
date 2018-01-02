#!/usr/bin/env bash

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

# Uses the kubekins-job-builder Docker image to compute the differences in
# the Jenkins job config XML, comparing the current git branch against master.
# The diff is printed at the end, assuming everything parsed successfully.

# Note: anecdotal evidence suggests this doesn't work correctly on OS X.
# If you find that there is no diff being generated, you may want to try setting
# OUTPUT_DIR to some directory in your home directory.

# When running this script from inside Docker, you must set REPO_ROOT to point
# to the path to the repository on the host, and DOCKER_VOLUME_OUTPUT_DIR must
# point to the path of $OUTPUT_DIR on the host. This is due to the way volume
# mounts work in Docker-in-Docker.

set -o errexit
set -o nounset
set -o pipefail

readonly JOB_CONFIGS_ROOT="jenkins/job-configs"
readonly JOB_BUILDER_IMAGE="k8s.gcr.io/kubekins-job-builder:5"

REPO_ROOT=$(cd $(dirname "${BASH_SOURCE}")/.. && pwd)
REPO_DIR=${REPO_DIR:-"${REPO_ROOT}"}

readonly output_dir=${OUTPUT_DIR:=$(mktemp -d -t JJB-XXXXX)}
readonly docker_volume_output_dir=${DOCKER_VOLUME_OUTPUT_DIR:="${output_dir}"}

mkdir -p "${output_dir}/upstream" "${output_dir}/patch"

echo "Saving output in ${output_dir}"

readonly common_commands="\
  git describe --long --tags --always --dirty --abbrev=14 >/output/gitversion.txt && \
  git rev-parse --abbrev-ref HEAD >/output/gitbranch.txt && \
  jenkins-jobs test \
    '${JOB_CONFIGS_ROOT}:${JOB_CONFIGS_ROOT}/kubernetes-jenkins' \
    -o /output/kubernetes-jenkins && \
  jenkins-jobs test \
    '${JOB_CONFIGS_ROOT}:${JOB_CONFIGS_ROOT}/kubernetes-jenkins-pull' \
    -o /output/kubernetes-jenkins-pull"

# We don't want to modify the local source in any way, so mount it read-only
# and checkout the master branch in a separate directory.
docker run --rm=true -i \
  -v "${REPO_DIR}:/test-infra:ro" \
  -v "${docker_volume_output_dir}/upstream:/output" \
  "${JOB_BUILDER_IMAGE}" \
  bash -c "git clone -b master --single-branch /test-infra /workspace && \
    cd /workspace && ${common_commands}"

docker run --rm=true -i \
  -v "${docker_volume_output_dir}/patch:/output" \
  -v "${REPO_DIR}:/test-infra:ro" \
  "${JOB_BUILDER_IMAGE}" \
  bash -c "${common_commands}"

readonly result_diff=$(diff -ruN "${output_dir}/upstream" "${output_dir}/patch" || true)
echo "${result_diff}"
if [[ "${TRAVIS:-}" != "true" ]]; then
  less -fF <(echo "${result_diff}")
fi
