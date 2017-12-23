#!/bin/bash
# Copyright 2015 The Kubernetes Authors.
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

desc "There are no claims"
run "kubectl --namespace=demos get pvc"

desc "Create a claim"
run "cat $(relative claim.yaml)"
run "kubectl --namespace=demos create -f $(relative claim.yaml)"

desc "Check it out"
run "kubectl --namespace=demos describe pvc pv-provisioning-demo"

desc "Wait for it to be satisfied"
run "while true; do
    X=\$(kubectl --namespace=demos get pvc pv-provisioning-demo);
    echo \"\$X\";
    echo \$X | grep Bound >/dev/null && break;
    sleep 1;
    done"

desc "You can see it in gcloud"
run "gcloud compute disks list | grep kubernetes-dynamic"

desc "Create a pod using the claim"
run "cat $(relative pod.yaml)"
run "kubectl --namespace=demos create -f $(relative pod.yaml)"

desc "Here's the pod"
run "kubectl --namespace=demos describe pods -l demo=pv-provisioning"

desc "Wait for it to be running"
run "while true; do
    X=\$(kubectl --namespace=demos get pods -l demo=pv-provisioning);
    echo \"\$X\";
    echo \$X | grep Running >/dev/null && break;
    sleep 1;
    done"

POD=$(kubectl --namespace=demos get pods -l demo=pv-provisioning -o name | cut -d/ -f2)
desc "Shell into it"
run "kubectl --namespace=demos exec --tty -i $POD sh"

desc "Kill the pod"
run "kubectl --namespace=demos delete pods -l demo=pv-provisioning"
run "kubectl --namespace=demos get pods -l demo=pv-provisioning"

desc "The claim still exists"
run "kubectl --namespace=demos describe pvc pv-provisioning-demo"

desc "The disk still exists"
run "gcloud compute disks list | grep kubernetes-dynamic"

desc "Run another pod using the same claim"
run "kubectl --namespace=demos create -f $(relative pod.yaml)"

desc "Wait for it to be running"
run "while true; do
    X=\$(kubectl --namespace=demos get pods -l demo=pv-provisioning);
    echo \"\$X\";
    echo \$X | grep Running >/dev/null && break;
    sleep 1;
    done"

POD=$(kubectl --namespace=demos get pods -l demo=pv-provisioning -o name | cut -d/ -f2)
desc "Shell into the new one"
run "kubectl --namespace=demos exec --tty -i $POD sh"

desc "Tear it down"
run "kubectl --namespace=demos delete pods -l demo=pv-provisioning"
run "kubectl --namespace=demos get pods -l demo=pv-provisioning"
run "kubectl --namespace=demos delete pvc -l demo=pv-provisioning"
run "kubectl --namespace=demos get pvc -l demo=pv-provisioning"
run "kubectl --namespace=demos get pv"
