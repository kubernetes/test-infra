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

# This script will gather all jenkins jobs in a PR and compare with target testgrid config.

set -o errexit
set -o nounset
set -o pipefail

docker run --rm=true \
  -w "/go/src/k8s.io/test-infra" \
  -v "${GOPATH}/src/k8s.io/test-infra:/go/src/k8s.io/test-infra" \
  'gcr.io/google_containers/kubekins-job-builder:5' \
  jenkins-jobs --ignore-cache test -o testgrid/output jenkins/job-configs:jenkins/job-configs/kubernetes-jenkins

docker run --rm=true \
  -w "/go/src/k8s.io/test-infra" \
  -v "${GOPATH}/src/k8s.io/test-infra:/go/src/k8s.io/test-infra" \
  'golang:1.7.1' \
  go run testgrid/jenkins_verify/jenkins_validate.go testgrid/output prow testgrid/config/config.yaml
