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

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

readonly testinfra="$(dirname "${0}")/.."

if [[ -z "${JENKINS_AWS_SSH_PRIVATE_KEY_FILE:-}" ]]; then
  echo "JENKINS_AWS_SSH_PRIVATE_KEY_FILE not set, assuming ${HOME}/.ssh/kube_aws_rsa"
  export JENKINS_AWS_SSH_PRIVATE_KEY_FILE="${HOME}/.ssh/kube_aws_rsa"
fi

if [[ -z "${JENKINS_AWS_SSH_PUBLIC_KEY_FILE:-}" ]]; then
  echo "JENKINS_AWS_SSH_PUBLIC_KEY_FILE not set, assuming ${HOME}/.ssh/kube_aws_rsa.pub"
  export JENKINS_AWS_SSH_PUBLIC_KEY_FILE="${HOME}/.ssh/kube_aws_rsa.pub"
fi

if [[ -z "${JENKINS_AWS_CREDENTIALS_FILE:-}" ]]; then
  echo "JENKINS_AWS_CREDENTIALS_FILE not set, assuming ${HOME}/.aws/credentials"
  export JENKINS_AWS_CREDENTIALS_FILE="${HOME}/.aws/credentials"
fi

### builder

# Fake provider to trick e2e-runner.sh
export KUBERNETES_PROVIDER="kops-aws"
export AWS_CONFIG_FILE="/workspace/.aws/credentials"
# This is needed to be able to create PD from the e2e test
export AWS_SHARED_CREDENTIALS_FILE="/workspace/.aws/credentials"
# TODO(zmerlynn): Eliminate the other uses of this env variable
export KUBE_SSH_USER=admin
export LOG_DUMP_USE_KUBECTL=yes
export LOG_DUMP_SSH_KEY=/workspace/.ssh/kube_aws_rsa
export LOG_DUMP_SSH_USER=admin
export LOG_DUMP_SAVE_LOGS="cloud-init-output"
export LOG_DUMP_SAVE_SERVICES="protokube"

### job-env

# See https://github.com/kubernetes/kubernetes/issues/30312 for why HPA is disabled.
# See https://github.com/kubernetes/node-problem-detector/issues/28 for why NPD is disabled.
# See https://github.com/kubernetes/kops/issues/774 for why the Dashboard is disabled
# See https://github.com/kubernetes/kops/issues/775 for why NodePort is disabled

DEFAULT_GINKGO_TEST_ARGS=\
  "--ginkgo.skip=\[Slow\]|\[Serial\]|\[Disruptive\]|\[Flaky\]|\[Feature:.+\]|\[HPA\]|NodeProblemDetector|Dashboard|Services.*functioning.*NodePort"
export GINKGO_TEST_ARGS="${GINKGO_TEST_ARGS:-${DEFAULT_GINKGO_TEST_ARGS}}"
if [[ -n "${JOB_NAME:-}" ]]; then
  # Running on Jenkins
  export KOPS_E2E_CLUSTER_NAME="e2e-kops-aws.test-aws.k8s.io"
  export KOPS_E2E_STATE_STORE="s3://k8s-kops-jenkins/"
  export KOPS_LATEST="latest-ci-updown-green.txt"
  export KOPS_PUBLISH_GREEN_PATH="gs://kops-ci/bin/latest-ci-green.txt"
else
  if [[ -z "${KOPS_E2E_CLUSTER_NAME:-}" ]]; then
    echo "KOPS_E2E_CLUSTER_NAME not set!" >&2
    exit 1
  fi
  if [[ -z "${KOPS_E2E_STATE_STORE:-}" ]]; then
    echo "KOPS_E2E_STATE_STORE not set!" >&2
    exit 1
  fi
  export WORKSPACE="${WORKSPACE:-$PWD}"
  echo "E2Es results will be output to ${WORKSPACE}/_artifacts"

  export JOB_NAME="${USER}"
  export BUILD_NUMBER=$(date +%s)
fi

### post-env

# Assume we're upping, testing, and downing a cluster
export E2E_UP="${E2E_UP:-true}"
export E2E_TEST="${E2E_TEST:-true}"
export E2E_DOWN="${E2E_DOWN:-true}"

# Skip gcloud update checking
export CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true
# Use default component update behavior
export CLOUDSDK_EXPERIMENTAL_FAST_COMPONENT_UPDATE=false

# Get golang into our PATH so we can run e2e.go
export PATH="${PATH}:/usr/local/go/bin"

# After post-env
export KOPS_DEPLOY_LATEST_KUBE=y
export KUBE_E2E_RUNNER="/workspace/kops-e2e-runner.sh"
export E2E_OPT="--kops-cluster ${KOPS_E2E_CLUSTER_NAME} --kops-state ${KOPS_E2E_STATE_STORE} --kops-nodes=4"
export GINKGO_PARALLEL="y"

### Runner
readonly runner="${testinfra}/jenkins/dockerized-e2e-runner.sh"
export DOCKER_TIMEOUT="140m"
export KUBEKINS_TIMEOUT="120m"
"${runner}"
