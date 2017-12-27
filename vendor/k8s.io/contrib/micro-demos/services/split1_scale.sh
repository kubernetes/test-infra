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

WHAT_WAS_RUN="$1"

desc "Resize the RC and watch the service backends change"
run "kubectl --namespace=demos scale $WHAT_WAS_RUN --replicas=1"
run "kubectl --namespace=demos scale $WHAT_WAS_RUN --replicas=2"
run "kubectl --namespace=demos scale $WHAT_WAS_RUN --replicas=5"

desc "Fire up a cloud load-balancer"
run "kubectl --namespace=demos get svc hostnames-svc -o yaml \\
    | sed 's/ClusterIP/LoadBalancer/' \\
    | kubectl replace -f -"
while true; do
    run "kubectl --namespace=demos get svc hostnames -o yaml | grep loadBalancer -A 4"
    if kubectl --namespace=demos get svc hostnames \
        -o go-template='{{index (index .status.loadBalancer.ingress 0) "ip"}}' \
        >/dev/null 2>&1; then
        break
    fi
done
