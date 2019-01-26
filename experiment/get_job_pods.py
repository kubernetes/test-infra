#!/usr/bin/env python

# Copyright 2017 The Kubernetes Authors.
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

# This script prints out lines with: "job-name: ['pod-id', 'pod-id-2']""
#
# USAGE: have KUBECONFIG pointed at your prow builds cluster then:
#
# get_job_pods.py [--show-all]
#
# EG:
# `experiment/get_job_pods.py --show-all | grep pull-kubernetes-bazel-test`
# will get you something like:
# pull-kubernetes-bazel-test: ['c9e634a5-cbe6-11e7-9149-0a580a6c0216']

from __future__ import print_function

from argparse import ArgumentParser
from collections import defaultdict
import json
import subprocess

def get_pods_json(show_all):
    cmd = ["kubectl", "get", "po", "-n=test-pods", "-o=json"]
    if show_all:
        cmd.append("--show-all")
    res = subprocess.check_output(cmd)
    return json.loads(res)["items"]

def get_pods_by_job(show_all):
    pods = get_pods_json(show_all)
    pods_by_job = defaultdict(list)
    for pod in pods:
        # check if prow job
        if "prow.k8s.io/job" not in pod["metadata"]["labels"]:
            continue
        # get pod and job name, add to dict
        pod_name = pod["metadata"]["name"].encode('utf-8')
        job_name = pod["metadata"]["labels"]["prow.k8s.io/job"].encode('utf-8')
        pods_by_job[job_name].append(pod_name)
    return pods_by_job

def main():
    parser = ArgumentParser()
    parser.add_argument("--show-all", action='store_true')
    args = parser.parse_args()
    # get and print pods for each job
    pods_by_job = get_pods_by_job(args.show_all)
    jobs = sorted(pods_by_job.keys())
    for job in jobs:
        print(job+":", pods_by_job[job])

if __name__ == '__main__':
    main()
