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
run "kubectl --namespace=demos run hostnames \\
    --image=gcr.io/google_containers/serve_hostname:1.1 \\
    --replicas=5 \\
    -o name"
WHAT_WAS_RUN="$DEMO_RUN_STDOUT"

desc "Look what I made!"
run "kubectl --namespace=demos describe $WHAT_WAS_RUN"

desc "These are the pods that were created"
run "kubectl --namespace=demos get pods -l run=hostnames"

IPS=($(kubectl --namespace=demos get pods -l run=hostnames \
          -o go-template='{{range .items}}{{.status.podIP}}{{"\n"}}{{end}}'))
desc "SSH into my cluster and access the pods"
run "kubectl --namespace=demos get pods -l run=hostnames \\
    -o go-template='{{range .items}}{{.status.podIP}}{{\"\\n\"}}{{end}}'"
run "gcloud compute ssh --zone=us-central1-b $SSH_NODE --command '\\
    for IP in ${IPS[*]}; do \\
        curl --connect-timeout 1 -s \$IP:9376 && echo; \\
    done \\
    '"

desc "Kill a pod"
VICTIM=$(kubectl --namespace=demos get pods -o name -l run=hostnames | tail -1)
run "kubectl --namespace=demos delete $VICTIM"
run "kubectl --namespace=demos get pods -l run=hostnames"
run "kubectl --namespace=demos describe $WHAT_WAS_RUN"

desc "Kill a node"
NODE=$(kubectl --namespace=demos get pods -l run=hostnames -o wide \
               | tail -1 \
               | awk '{print $NF}')
run "kubectl --namespace=demos get pods -l run=hostnames -o wide"
run "gcloud compute ssh --zone=us-central1-b $NODE --command '\\
    sudo shutdown -r now; \\
    '"
while true; do
    run "kubectl --namespace=demos get node $NODE"
    status=$(kubectl --namespace=demos get node $NODE | tail -1 | awk '{print $2}')
    if [ "$status" == "NotReady" ]; then
        break
    fi
done
run "kubectl --namespace=demos get pods -l run=hostnames -o wide"
run "kubectl --namespace=demos describe $WHAT_WAS_RUN"
