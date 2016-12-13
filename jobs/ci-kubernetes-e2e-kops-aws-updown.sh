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
export LOG_DUMP_SAVE_LOGS=cloud-init-output

### job-env
export E2E_NAME="e2e-kops-aws-updown"
export GINKGO_TEST_ARGS="--ginkgo.focus=\[k8s.io\]\sNetworking.*\[Conformance\]"
export KOPS_PUBLISH_GREEN_PATH="gs://kops-ci/bin/latest-ci-updown-green.txt"

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
# TODO(zmerlynn): Take out --kops-ssh-key after fixing kops-e2e-runner again.
export E2E_OPT="--kops-ssh-key /workspace/.ssh/kube_aws_rsa --kops-cluster ${E2E_NAME}.test-aws.k8s.io --kops-state s3://k8s-kops-jenkins/ --kops-nodes=4"
export GINKGO_PARALLEL="y"

### Runner
readonly runner="${testinfra}/jenkins/dockerized-e2e-runner.sh"
export KUBEKINS_TIMEOUT="30m"
"${runner}"
