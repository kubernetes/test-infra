#!/usr/bin/env python3

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

# This script hijacks the COS kubelet service definition to set soft eviction thresholds
# for nodefs on the prow builds cluster
# USE AT YOUR OWN RISK.
# TODO: delete this once dynamic kubelet config is available

# pylint: disable=line-too-long


import os
import sys
import subprocess

# xref prow/Makefile get-build-cluster-credentials
CLUSTER = 'prow'
ZONE = 'us-central1-f'
PROJECT = 'k8s-prow-builds'

AUTH_TO_CLUSTER_COMMAND = 'gcloud container clusters get-credentials %s --project=%s --zone=%s' % (CLUSTER, PROJECT, ZONE)

# this should be 20% more than the hard eviction threshold
# the grace period should be longer than the typical time for another pod to be cleaned up by sinker
KUBELET_ARGS_TO_ADD = '--eviction-soft=nodefs.available<30% --eviction-soft-grace-period=nodefs.available=2h'
# commands used *in order* to update the kubelet
KUBELET_UPDATE_COMMANDS = [
    # this works because the ExecStart line normally ends with $KUBELET_OPTS
    # so we replace `KUBELET_OPTS.*` (to the end of the line) with `KUBELET_OPTS --some --args ---we --want`
    "sudo sed -i 's/KUBELET_OPTS.*/KUBELET_OPTS %s/' /etc/systemd/system/kubelet.service" % KUBELET_ARGS_TO_ADD,
    "sudo systemctl daemon-reload",
    "sudo systemctl restart kubelet"
]

def get_nodes():
    command = ['kubectl', 'get', 'nodes']
    res = subprocess.check_output(command, encoding='utf-8')
    nodes = []
    for line in res.split('\n')[1:]:
        node = line.split(' ')[0]
        if node != '':
            nodes.append(node)
    return nodes


def run_on_node(node_name, command):
    print("node: %s running: %s" % (node_name, command))
    subprocess.call(['gcloud', 'compute', 'ssh', '--project='+PROJECT, '--zone='+ZONE, '--command='+command, node_name])

def main():
    if sys.argv[-1] != "--yes-i-accept-that-this-is-very-risky":
        print("This command is very risky and unsupported (!)")
        print("Do not run this unless you know what you are doing and accept the consequences (!)")
        sys.exit(-1)

    # auth to the cluster
    print('getting cluster auth...')
    os.system(AUTH_TO_CLUSTER_COMMAND)
    print('')

    # get the list of nodes
    print('getting nodes...')
    nodes = get_nodes()
    print("got %d nodes." % len(nodes))
    print('')

    # run our service patch command on the nodes
    print('updating kubelet service on the nodes...')
    for node in nodes:
        print("\nupdating node: %s" % node)
        for command in KUBELET_UPDATE_COMMANDS:
            run_on_node(node, command)

    print('\ndone')

if __name__ == '__main__':
    main()
