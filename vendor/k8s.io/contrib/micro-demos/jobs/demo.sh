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

desc "Run some pods in a job"
run "cat $(relative job.yaml)"
run "kubectl --namespace=demos create -f $(relative job.yaml)"

desc "See what we did"
run "kubectl --namespace=demos describe job jobs-demo"

desc "See pods run"
while [ "$(kubectl --namespace=demos get job jobs-demo -o go-template='{{.status.succeeded}}')" != 15 ]; do
	run "kubectl --namespace=demos get pods -l demo=jobs"
	run "kubectl --namespace=demos describe job jobs-demo"
done

desc "Final status"
run "kubectl --namespace=demos get pods --show-all -l demo=jobs --sort-by='{.status.phase}'"
