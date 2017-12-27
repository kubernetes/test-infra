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

desc "Create a pod"
run "cat $(relative pod.yaml)"
run "kubectl --namespace=demos create -f $(relative pod.yaml)"

desc "Hey look, a pod!"
run "kubectl --namespace=demos get pods"

desc "Get the pod's logs"
run "kubectl --namespace=demos logs graceful-demo --follow"

desc "Delete the pod"
run "kubectl --namespace=demos delete pod graceful-demo"
run "kubectl --namespace=demos get pods graceful-demo"

desc "Get the pod's logs"
run "kubectl --namespace=demos logs graceful-demo --follow"
