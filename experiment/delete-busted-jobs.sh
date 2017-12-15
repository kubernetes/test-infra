#!/usr/bin/env bash
# Copyright 2017 The Kubernetes Authors.
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

TESTINFRA_ROOT=$(git rev-parse --show-toplevel)
cd $TESTINFRA_ROOT

make -C prow get-build-cluster-credentials
BUSTED_PODS="$(kubectl get po -n=test-pods | grep -i imagepull | cut -d " " -f1)"
echo "Busted pods are:"
for pod in $BUSTED_PODS; do
    echo $pod
done
make -C prow get-cluster-credentials
for pod in $BUSTED_PODS; do
    kubectl delete prowjob $pod
done
make -C prow get-build-cluster-credentials
for pod in $BUSTED_PODS; do
    kubectl delete pod -n=test-pods $pod
done
