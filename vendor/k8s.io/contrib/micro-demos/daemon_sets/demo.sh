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

for NODE in $(kubectl get nodes -o name | cut -f2 -d/); do
    kubectl label node $NODE color- --overwrite >/dev/null 2>&1
done

desc "No labels on nodes"
run "kubectl get nodes \\
    -o go-template='{{range .items}}{{.metadata.name}}{{\"\t\"}}{{.metadata.labels}}{{\"\n\"}}{{end}}'"

desc "Run a service to front our daemon"
run "cat $(relative svc.yaml)"
run "kubectl --namespace=demos create -f $(relative svc.yaml)"

desc "Run our daemon"
run "cat $(relative daemon.yaml)"
run "kubectl --namespace=demos create -f $(relative daemon.yaml) --validate=false"
run "kubectl --namespace=demos describe ds daemons-demo"

tmux new -d -s my-session \
    "$(dirname ${BASH_SOURCE})/split1_color_nodes.sh" \; \
    split-window -v -d "$(dirname $BASH_SOURCE)/split1_hit_svc.sh" \; \
    attach \;
