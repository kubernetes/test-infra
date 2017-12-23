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

desc "Run some pods"
run "kubectl --namespace=demos run hostnames-svc \\
    --image=gcr.io/google_containers/serve_hostname:1.1 \\
    --replicas=5 \\
    -o name"
WHAT_WAS_RUN="$DEMO_RUN_STDOUT"

desc "Expose the result as a service"
run "kubectl --namespace=demos expose $WHAT_WAS_RUN \\
    --port=80 --target-port=9376"

desc "Have a look at the service"
run "kubectl --namespace=demos describe svc hostnames-svc"

IP=$(kubectl --namespace=demos get svc hostnames-svc \
    -o go-template='{{.spec.clusterIP}}')
desc "See what happens when you access the service's IP"
run "gcloud compute ssh --zone=us-central1-b $SSH_NODE --command '\\
    for i in \$(seq 1 10); do \\
        curl --connect-timeout 1 -s $IP && echo; \\
    done \\
    '"
run "gcloud compute ssh --zone=us-central1-b $SSH_NODE --command '\\
    for i in \$(seq 1 500); do \\
        curl --connect-timeout 1 -s $IP && echo; \\
    done | sort | uniq -c; \\
    '"

tmux new -d -s my-session \
    "sleep 10; $(dirname ${BASH_SOURCE})/split1_scale.sh $WHAT_WAS_RUN" \; \
    split-window -h -d "$(dirname $BASH_SOURCE)/split1_hit_svc.sh $WHAT_WAS_RUN" \; \
    attach \;
