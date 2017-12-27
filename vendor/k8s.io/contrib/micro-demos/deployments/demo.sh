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

desc "Create a service that fronts any version of this demo"
run "cat $(relative svc.yaml)"
run "kubectl --namespace=demos apply -f $(relative svc.yaml)"

desc "Deploy v1 of our app"
run "cat $(relative deployment.yaml)"
run "kubectl --namespace=demos apply -f $(relative deployment.yaml)"

# The output of describe is too wide, uncomment the following if needed.
# desc "Check it"
# run "kubectl --namespace=demos describe deployment deployment-demo"

tmux new -d -s my-session \
    "$(dirname $BASH_SOURCE)/split1_control.sh" \; \
    split-window -v -p 66 "$(dirname ${BASH_SOURCE})/split1_hit_svc.sh" \; \
    split-window -v "$(dirname ${BASH_SOURCE})/split1_watch.sh v1" \; \
    split-window -h -d "$(dirname ${BASH_SOURCE})/split1_watch.sh v2" \; \
    select-pane -t 0 \; \
    attach \;
