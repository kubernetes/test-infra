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

desc "Run some pods under a replication controller"
run "kubectl --namespace=demos run yes-autoscaler-demo \\
    --image=busybox \\
    --replicas=1 \\
    --limits=cpu=100m \\
    -o name \\
    -- sh -c 'sleep 5; yes > /dev/null'"
WHAT_WAS_RUN="$DEMO_RUN_STDOUT"

desc "Look what I made!"
run "kubectl --namespace=demos describe $WHAT_WAS_RUN"

desc "One pod was created"
run "kubectl --namespace=demos get pods -l run=yes-autoscaler-demo"

desc "Create a pod autoscaler"
run "kubectl --namespace=demos autoscale $WHAT_WAS_RUN --min=1 --max=10 --cpu-percent=25"

desc "Watch pods get created"
while true; do
    run "kubectl --namespace=demos describe hpa yes-autoscaler-demo"
    run "kubectl --namespace=demos get pods -l run=yes-autoscaler-demo"
done
