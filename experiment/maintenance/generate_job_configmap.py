#!/usr/bin/env python

# Copyright 2018 The Kubernetes Authors.
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

# This script deletes and recreates the job configmap

#
# USAGE: have KUBECONFIG pointed at your prow cluster then from test-infra root:
#
# hack/generate-job-configmap.py [--wet]
#

from __future__ import print_function

from argparse import ArgumentParser
import os
import subprocess

def main():
    parser = ArgumentParser()
    parser.add_argument("--job-configmap", default="job-config", help="name of prow jobs configmap")
    parser.add_argument(
        "--job-config-dir", default="config/jobs", help="root dir of prow jobs configmap")
    parser.add_argument("--wet", action="store_true")

    args = parser.parse_args()

    # delete configmap (apply has size limit)
    cmd = ["kubectl", "delete", "configmap", args.job_configmap]
    print (cmd)
    if args.wet:
        subprocess.check_call(cmd)

    # regenerate
    cmd = ["kubectl", "create", "configmap", args.job_configmap]
    for root, _, files in os.walk(args.job_config_dir):
        for name in files:
            if name.endswith(".yaml"):
                cmd.append("--from-file=%s=%s" % (name, os.path.join(root, name)))
    print (cmd)
    if args.wet:
        subprocess.check_call(cmd)


if __name__ == '__main__':
    main()
