#!/usr/bin/env python

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

"""Script to snapshot a jenkins master or restore those snapshots.

Note that this does not clean up old snapshots, nor does it handle problem
(for example duplicate build numbers) associated with restoring an old
snapshot. Thus the restore feature should be used in emergencies only.
"""

import argparse
import getpass
import subprocess
import sys
import time


def gcloud(project, *args, **kwargs):
    cmd = ('gcloud', '--project=%s' % project, 'compute') + args
    print >>sys.stderr, 'RUN:', ' '.join(cmd)
    if not kwargs.get('output'):
        subprocess.check_call(cmd)
        return ''
    return subprocess.check_output(cmd)


def snapshot_disks(project, zone, *disks):
    ymd = time.strftime('%Y%m%d', time.localtime())
    snapshots = ['%s-%s' % (d, ymd) for d in disks]
    gcloud(
        project,
        'disks',
        'snapshot',
        '--zone=%s' % zone,
        '--snapshot-names=%s' % ','.join(snapshots),
        *disks)


def get_disks(instance):
    """Return a {name: size_gb} map."""
    return {
        instance: 500,
        '%s-data' % instance: 1000,
        '%s-docker' % instance: 1000,  # Only 200 in pr
    }


def get_address(project, zone, instance):
    """Return the reserved ip address of the instance."""
    return gcloud(
        project,
        'addresses',
        'describe',
        '%s-ip' % instance,
        '--region=%s' % get_region(zone),
        '--format=value(address)',
    )



def get_region(zone):
    """Converts us-central1-f into us-central1."""
    return '-'.join(zone.split('-')[:2])


def get_snapshot(project, zone, instance):
    """Snapshot all the disks for this instance."""
    snapshot_disks(project, zone, *get_disks(instance))


def delete(project, zone, instance):
    """Confirm and delete instance and associated disks."""
    print >>sys.stderr, 'WARNING: duplicated jobs may fail/corrupt results'
    print >>sys.stderr, ('TODO(fejta): See http://stackoverflow.com/'
                         'questions/19645430/changing-jenkins-build-number')
    answer = raw_input('Delete %s [yes/NO]: ')
    if not answer or answer != 'yes':
        print >>sys.stderr, 'aborting'
        sys.exit(1)
    gcloud(
        project,
        'compute',
        'instances',
        'delete',
        '--zone=%s' % zone,
        instance,
    )
    gcloud(
        project,
        'compute',
        'disks',
        'delete',
        '--zone=%s' % zone,
        *get_disks(instance))


SCOPES = [
    'cloud-platform',
    'compute-rw',
    'logging-write',
    'storage-rw',
]


def restore(project, zone, instance, snapshot):
    """Restore instance and disks from the snapshot suffix."""
    disks = []
    description = 'Created from %s by %s' % (snapshot, getpass.getuser())
    for disk, size in get_disks(instance).items():
        gcloud(
            project,
            'compute',
            'disks',
            'create',
            '--description=%s' % description,
            '--zone=%s' % zone,
            '--size=%dGB' % size,
            '--source-snapshot=%s-%s' % (disk, snapshot),
            disk,
        )
        attrs = [
            'name=%s' % disk,
            'device-name=%s' % disk,
        ]
        if disk == instance:
            attrs.append('boot=yes')
        disks.append(attrs)
    gcloud(
        project,
        'compute',
        'instances',
        'create',
        '--description=%s' % description,
        '--zone=%s' % zone,
        '--machine-type=n1-highmem-32',  # should reduce to 8
        '--scopes=%s' % ','.join(SCOPES),
        '--tag=do-not-delete,jenkins,jenkins-master',
        '--address=%s' % get_address(project, zone, instance),
        *('--disk=%s' % ','.join(a) for a in disks))



def main(args):
    if args.pr:
        project, instance = 'kubernetes-jenkins-pull', 'pull-jenkins-master'
    else:
        project, instance = 'kubernetes-jenkins', 'jenkins-master'
    zone = args.zone
    if not args.restore:
        get_snapshot(project, zone, instance)
        return
    if args.delete:
        delete(project, zone, instance)
    snapshot = args.restore
    restore(project, zone, instance, snapshot)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser('Tool to backup/restore the jenkins master')
    PARSER.add_argument(
        '--zone', default='us-central1-f', help='Jenkins zone')
    PARSER.add_argument(
        '--pr', action='store_true', help='Manipulate PR jenkins when set')
    PARSER.add_argument(
        '--restore', help='restore jenkins to the YYYYMMDD snapshot when set')
    PARSER.add_argument(
        '--delete', help='delete current jenkins instance before restoring')
    ARGS = PARSER.parse_args()
    main(ARGS)
