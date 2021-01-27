#!/usr/bin/env python3

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

# This script drains nodes from one node pool and adds nodes to another n:m at a time
#
# Use like:
# shift_nodepool_capacity.py pool-to-drain pool-to-grow shrink_increment:grow_increment num_to_add
#
# EG:
# shift_nodepool_capacity.py 5 2:1 default-pool pool-n1-highmem-8-300gb
#
# for nodefs on the prow builds cluster
# USE AT YOUR OWN RISK.

import argparse
import sys
import subprocess
import json
import math


def get_pool_sizes(project, zone, cluster):
    """returns a map of node pool name to size using the gcloud cli."""
    sizes = {}

    # map managed instance group names to node pools and record pool names
    node_pools = json.loads(subprocess.check_output([
        'gcloud', 'container', 'node-pools', 'list',
        '--project', project, '--cluster', cluster, '--zone', zone,
        '--format=json',
    ], encoding='utf-8'))
    group_to_pool = {}
    for pool in node_pools:
        # later on we will sum up node counts from instance groups
        sizes[pool['name']] = 0
        # this is somewhat brittle, the last component of the URL is the instance group name
        # the better way to do this is probably to use the APIs directly
        for url in pool['instanceGroupUrls']:
            instance_group = url.split('/')[-1]
            group_to_pool[instance_group] = pool['name']

    # map instance groups to node counts
    groups = json.loads(subprocess.check_output([
        'gcloud', 'compute', 'instance-groups', 'list',
        '--project', project, '--filter=zone:({})'.format(zone),
        '--format=json',
    ], encoding='utf-8'))
    for group in groups:
        if group['name'] not in group_to_pool:
            continue
        sizes[group_to_pool[group['name']]] += group['size']

    return sizes


def resize_nodepool(pool, new_size, project, zone, cluster):
    """resize the nodepool to new_size using the gcloud cli"""
    cmd = [
        'gcloud', 'container', 'clusters', 'resize', cluster,
        '--zone', zone, '--project', project, '--node-pool', pool,
        '--num-nodes', str(new_size), '--quiet',
    ]
    print(cmd)
    subprocess.call(cmd)


def prompt_confirmation():
    """prompts for interactive confirmation, exits 1 unless input is 'yes'"""
    sys.stdout.write('Please confirm (yes/no): ')
    response = input()
    if response != 'yes':
        print('Cancelling.')
        sys.exit(-1)
    print('Confirmed.')


# xref prow/Makefile get-build-cluster-credentials
def parse_args(args):
    parser = argparse.ArgumentParser()
    parser.add_argument('nodes', type=int,
                        help='Number of Nodes to add.')
    parser.add_argument('ratio', type=str,
                        help='ShrinkIncrement:GrowIncrement, Ex 2:1.')
    parser.add_argument('shrink', type=str,
                        help='Pool name to drain nodes from.')
    parser.add_argument('grow', type=str,
                        help='Pool name to grow nodes into.')
    parser.add_argument('--cluster', type=str, default="prow",
                        help='Name of GCP cluster.')
    parser.add_argument('--zone', type=str, default='us-central1-f',
                        help='GCP zonal location of the cluster.')
    parser.add_argument('--project', type=str, default='k8s-prow-builds',
                        help='GCP Project that the cluster exists within.')
    return parser.parse_args(args)


def main(options):
    # parse cli
    nodes_to_add = options.nodes

    ratio = options.ratio.split(':')
    shrink_increment, grow_increment = int(ratio[0]), int(ratio[1])

    pool_to_grow = options.grow
    pool_to_shrink = options.shrink

    # obtain current pool sizes
    project, zone, cluster = options.project, options.zone, options.cluster
    pool_sizes = get_pool_sizes(project, zone, cluster)
    pool_to_grow_initial = pool_sizes[pool_to_grow]
    pool_to_shrink_initial = pool_sizes[pool_to_shrink]

    # compute final pool sizes
    pool_to_grow_target = pool_to_grow_initial + nodes_to_add

    n_iter = int(math.ceil(float(nodes_to_add) / grow_increment))
    pool_to_shrink_target = pool_to_shrink_initial - n_iter*shrink_increment
    if pool_to_shrink_target < 0:
        pool_to_shrink_target = 0

    # verify with the user
    print((
        'Shifting NodePool capacity for project = "{project}",'
        'zone = "{zone}", cluster = "{cluster}"'
        ).format(
            project=project, zone=zone, cluster=cluster,
        ))
    print('')
    print((
        'Will add {nodes_to_add} node(s) to {pool_to_grow}'
        ' and drain {shrink_increment} node(s) from {pool_to_shrink}'
        ' for every {grow_increment} node(s) added to {pool_to_grow}'
        ).format(
            nodes_to_add=nodes_to_add, shrink_increment=shrink_increment,
            grow_increment=grow_increment, pool_to_grow=pool_to_grow,
            pool_to_shrink=pool_to_shrink,
        ))
    print('')
    print((
        'Current pool sizes are: {{{pool_to_grow}: {pool_to_grow_curr},'
        ' {pool_to_shrink}: {pool_to_shrink_curr}}}'
        ).format(
            pool_to_grow=pool_to_grow, pool_to_grow_curr=pool_to_grow_initial,
            pool_to_shrink=pool_to_shrink, pool_to_shrink_curr=pool_to_shrink_initial,
        ))
    print('')
    print((
        'Target pool sizes are: {{{pool_to_grow}: {pool_to_grow_target},'
        ' {pool_to_shrink}: {pool_to_shrink_target}}}'
        ).format(
            pool_to_grow=pool_to_grow, pool_to_grow_target=pool_to_grow_target,
            pool_to_shrink=pool_to_shrink, pool_to_shrink_target=pool_to_shrink_target,
        ))
    print('')

    prompt_confirmation()
    print('')


    # actually start resizing
    # ignore pylint, "i" is a perfectly fine variable name for a loop counter...
    # pylint: disable=invalid-name
    for i in range(n_iter):
        # shrink by one increment, capped at reaching zero nodes
        print('Draining {shrink_increment} node(s) from {pool_to_shrink} ...'.format(
            shrink_increment=shrink_increment, pool_to_shrink=pool_to_shrink,
        ))
        new_size = max(pool_to_shrink_initial - (i*shrink_increment + shrink_increment), 0)
        resize_nodepool(pool_to_shrink, new_size, project, zone, cluster)
        print('')

        # ditto for growing, modulo the cap
        num_to_add = min(grow_increment, pool_to_grow_target - i*grow_increment)
        print('Adding {num_to_add} node(s) to {pool_to_grow} ...'.format(
            num_to_add=num_to_add, pool_to_grow=pool_to_grow,
        ))
        new_size = pool_to_grow_initial + (i*grow_increment + num_to_add)
        resize_nodepool(pool_to_grow, new_size, project, zone, cluster)
        print('')

    print('')
    print('Done')

if __name__ == '__main__':
    main(parse_args(sys.argv[1:]))
