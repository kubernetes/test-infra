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

# This script drains nodes from one node pool and adds nodes to another n:m at a time
#
# Use like:
# shift_nodepool_capacity.py pool-to-drain pool-to-grow shrink_increment:grow_increment num_to_add
#
# EG:
# shift_nodepool_capacity.py default-pool pool-n1-highmem-8-300gb 2:1 5
#
# for nodefs on the prow builds cluster
# USE AT YOUR OWN RISK.
# TODO(bentheelder): delete this once dynamic kubelet config is available


from __future__ import print_function

import sys
import subprocess
import json
import math

# xref prow/Makefile get-build-cluster-credentials
# TODO(bentheelder): perhaps make these configurable
CLUSTER = 'prow'
ZONE = 'us-central1-f'
PROJECT = 'k8s-prow-builds'


def get_pool_sizes(project, zone, cluster):
    """returns a map of node pool name to size using the gcloud cli."""
    sizes = {}

    # map managed instance group names to node pools and record pool names
    node_pools = json.loads(subprocess.check_output([
        'gcloud', 'container', 'node-pools', 'list',
        '--project', project, '--cluster', cluster, '--zone', zone,
        '--format=json',
    ]))
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
    ]))
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
        '--size', str(new_size), '--quiet',
    ]
    print(cmd)
    subprocess.call(cmd)


def prompt_confirmation():
    """prompts for interactive confirmation, exits 1 unless input is 'yes'"""
    sys.stdout.write('Please confirm (yes/no): ')
    response = raw_input()
    if response != 'yes':
        print('Cancelling.')
        sys.exit(-1)
    print('Confirmed.')


def main():
    # parse cli
    nodes_to_add = int(sys.argv[-1])

    ratio = sys.argv[-2].split(':')
    shrink_increment, grow_increment = int(ratio[0]), int(ratio[1])

    pool_to_grow = sys.argv[-3]
    pool_to_shrink = sys.argv[-4]

    # obtain current pool sizes
    pool_sizes = get_pool_sizes(PROJECT, ZONE, CLUSTER)
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
            project=PROJECT, zone=ZONE, cluster=CLUSTER,
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
        resize_nodepool(pool_to_shrink, new_size, PROJECT, ZONE, CLUSTER)
        print('')

        # ditto for growing, modulo the cap
        num_to_add = min(grow_increment, pool_to_grow_target - i*grow_increment)
        print('Adding {num_to_add} node(s) to {pool_to_grow} ...'.format(
            num_to_add=num_to_add, pool_to_grow=pool_to_grow,
        ))
        new_size = pool_to_grow_initial + (i*grow_increment + num_to_add)
        resize_nodepool(pool_to_grow, new_size, PROJECT, ZONE, CLUSTER)
        print('')

    print('')
    print('Done')

if __name__ == '__main__':
    main()
