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

. $(dirname ${BASH_SOURCE})/../util.sh

desc "Create a secret"
run "cat $(relative secret.yaml)"
run "kubectl --namespace=demos create -f $(relative secret.yaml)"

desc "Create a pod which uses that secret"
run "cat $(relative pod.yaml)"
run "kubectl --namespace=demos create -f $(relative pod.yaml)"

while true; do
    run "kubectl --namespace=demos get pod secrets-demo"
    status=$(kubectl --namespace=demos get pod secrets-demo | tail -1 | awk '{print $3}')
    if [ "$status" == "Running" ]; then
        break
    fi
done
run "kubectl --namespace=demos exec --tty -i secrets-demo sh"
