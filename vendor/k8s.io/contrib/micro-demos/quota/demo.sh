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

desc "There is no quota"
run "kubectl --namespace=demos get quota"

desc "Install quota"
run "cat $(relative quota.yaml)"
run "kubectl --namespace=demos create -f $(relative quota.yaml)"
run "kubectl --namespace=demos describe quota demo-quota"

desc "Create a large pod - should fail"
run "cat $(relative pod1.yaml)"
run "kubectl --namespace=demos create -f $(relative pod1.yaml)"
run "kubectl --namespace=demos describe quota demo-quota"

desc "Create a pod with no limits - should fail"
run "cat $(relative pod2.yaml)"
run "kubectl --namespace=demos create -f $(relative pod2.yaml)"
run "kubectl --namespace=demos describe quota demo-quota"

desc "There are no default limits"
run "kubectl --namespace=demos get limits"

desc "Set default limits"
run "cat $(relative limits.yaml)"
run "kubectl --namespace=demos create -f $(relative limits.yaml)"
run "kubectl --namespace=demos describe limits demo-limits"

desc "Create a pod with no limits - should succeed now"
run "cat $(relative pod2.yaml)"
run "kubectl --namespace=demos create -f $(relative pod2.yaml)"
run "kubectl --namespace=demos describe quota demo-quota"
