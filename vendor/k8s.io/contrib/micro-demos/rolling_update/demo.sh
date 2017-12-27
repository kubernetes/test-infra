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
run "kubectl --namespace=demos create -f $(relative svc.yaml)"

desc "Run v1 of our app"
run "cat $(relative rc-v1.yaml)"
run "kubectl --namespace=demos create -f $(relative rc-v1.yaml)"

tmux new -d -s my-session \
    "sleep 10; $(dirname ${BASH_SOURCE})/split1_update.sh" \; \
    split-window -h -d "$(dirname $BASH_SOURCE)/split1_hit_svc.sh" \; \
    attach \;
