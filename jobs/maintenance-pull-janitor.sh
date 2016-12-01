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

projects=(
  kubernetes-pr-cri-validation
  k8s-jkns-pr-kubemark
  k8s-jkns-pr-gce
  k8s-jkns-pr-gci-gce
  k8s-jkns-pr-gke
  k8s-jkns-pr-gci-gke
  k8s-jkns-pr-gci-kubemark
  k8s-jkns-pr-bldr-e2e-gce-fdrtn
  k8s-jkns-pr-gce-etcd3
  k8s-jkns-pr-gci-bld-e2e-gce-fd
)

./jenkins/janitor.py --project=sen-lu-test --hours=1

for proj in "${projects[@]}"; do
  ./jenkins/janitor.py --project="${proj}" --hours=24
done

