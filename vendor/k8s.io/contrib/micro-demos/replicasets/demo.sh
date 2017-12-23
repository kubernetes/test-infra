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

desc "Run some pods under a replica set"
run "cat $(relative rs.yaml)"
run "kubectl --namespace=demos create -f $(relative rs.yaml) --validate=false"
desc "Look what I made!"
run "kubectl --namespace=demos get replicasets"

desc "These are the pods that were created"
run "kubectl --namespace=demos get pods -l run=hostnames"

trap "" SIGINT

desc "Kill a pod"
VICTIM=$(kubectl --namespace=demos get pods -o name -l run=hostnames | tail -1)
run "kubectl --namespace=demos delete $VICTIM"
run "kubectl --namespace=demos get pods -l run=hostnames"

desc "Check on which nodes the pods are running"
run "kubectl --namespace=demos get pods -l run=hostnames -o wide"
desc "Kill a node"
NODE=$(kubectl --namespace=demos get pods -l run=hostnames -o wide \
               | tail -1 \
               | awk '{print $NF}')

run "gcloud compute ssh --zone=us-central1-b $NODE --command '\\
    sudo shutdown -r now; \\
    '"
while true; do
    run "kubectl --namespace=demos get node $NODE"
    # TODO: It's possible the two runs get different results. Need to get the output of run.
    status=$(kubectl --namespace=demos get node $NODE | tail -1 | awk '{print $2}')
    if [ "$status" == "NotReady" ]; then
        break
    fi
done

while true; do
    run "kubectl --namespace=demos get pods -l run=hostnames -o wide"
    pods_on_restarting_node=$(kubectl --namespace=demos get pods -l run=hostnames -o wide | grep $NODE)
    if [ -z "${pods_on_restarting_node}" ]; then
        break
    fi
done
