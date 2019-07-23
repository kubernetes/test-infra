#!/usr/bin/env bash
# Copyright 2019 The Kubernetes Authors.
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

job=${1:-pull-test-infra-yamllint}
config=${CONFIG_PATH:-$(readlink -f $(dirname "${BASH_SOURCE[0]}")/../../config.yaml)}
job_config=${JOB_CONFIG_PATH-$(readlink -f $(dirname "${BASH_SOURCE[0]}")/../../../config/jobs)}

echo job=${job}
echo config=${config}
echo job_config=${job_config}

if [[ -n ${job_config} ]]
then
  job_config="--job-config-path=${job_config}"
fi

found="false"
for clust in $(kind get clusters)
do
  if [[ ${clust} == "mkpod" ]]
  then
    found="true"
  fi
done

if [[ ${found} == "false" ]]
then
  kind create cluster --name=mkpod --config=local-kind-config.yaml --wait=5m
fi
export KUBECONFIG="$(kind get kubeconfig-path --name="mkpod")"

bazel run //prow/cmd/mkpj -- --config-path=${config} ${job_config} --job=${job} > ${PWD}/pj.yaml
bazel run //prow/cmd/mkpod -- --build-id=snowflake --prow-job=${PWD}/pj.yaml --local --out-dir=/output/${job} > ${PWD}/pod.yaml

pod=$(kubectl apply -f ${PWD}/pod.yaml | cut -d ' ' -f 1)
kubectl get ${pod} -w
