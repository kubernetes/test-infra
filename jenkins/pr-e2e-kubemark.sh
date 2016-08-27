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

set -ex
checkout="${PWD}/test-infra/jenkins/checkout.py"
runner="${PWD}/test-infra/jenkins/dockerized-e2e-runner.sh"
mkdir -p go/src/k8s.io
cd go/src/k8s.io
"${checkout}" --repo=kubernetes --pr="${grprbPullId}"
cd kubernetes

export kube_skip_push_gcs=y
export kube_run_from_output=y
export kube_fastbuild=true
./hack/jenkins/build.sh

# nothing should want jenkins $home
export home=${workspace}

export kubernetes_provider="gce"
# having full "kubemark" in name will result in exceeding allowed length
# of firewall-rule name.
export e2e_name="k6k-e2e-${node_name}-${executor_number}"
export project="k8s-jkns-pr-kubemark"
export e2e_up="true"
export e2e_test="false"
export e2e_down="true"
export use_kubemark="true"
export kubemark_tests="starting\s30\spods\sper\snode"
export fail_on_gcp_resource_leak="false"
# override defaults to be independent from gce defaults and set kubemark parameters
export num_nodes="1"
export master_size="n1-standard-1"
export node_size="n1-standard-2"
export kube_gce_zone="us-central1-f"
export kubemark_master_size="n1-standard-1"
export kubemark_num_nodes="5"
# the kubemark scripts build a docker image
export jenkins_enable_docker_in_docker="y"

# gce variables
export instance_prefix=${e2e_name}
export kube_gce_network=${e2e_name}
export kube_gce_instance_prefix=${e2e_name}

# force to use container-vm.
export kube_node_os_distribution="debian"
# skip gcloud update checking
export cloudsdk_component_manager_disable_update_check=true

# get golang into our path so we can run e2e.go
export path=${path}:/usr/local/go/bin

timeout -k 15m 45m {runner} && rc=$? || rc=$?
if [[ ${rc} -ne 0 ]]; then
  if [[ -x cluster/log-dump.sh && -d _artifacts ]]; then
    echo "dumping logs for any remaining nodes"
    ./cluster/log-dump.sh _artifacts
  fi
fi
if [[ ${rc} -eq 124 || ${rc} -eq 137 ]]; then
  echo "build timed out" >&2
elif [[ ${rc} -ne 0 ]]; then
  echo "build failed" >&2
fi
echo "exiting with code: ${rc}"
exit ${rc}
