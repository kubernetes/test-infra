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

"""Clean up resources from gcp projects. """

import argparse
import collections
import datetime
import json
import subprocess
import sys


# A resource that need to be cleared.
Resource = collections.namedtuple('Resource', 'name group condition managed')
DEMOLISH_ORDER = [
    # Beware of insertion order
    Resource('instances', None, 'zone', None),
    Resource('addresses', None, 'region', None),
    Resource('disks', None, 'zone', None),
    Resource('firewall-rules', None, None, None),
    Resource('routes', None, None, None),
    Resource('forwarding-rules', None, 'region', None),
    Resource('target-http-proxies', None, None, None),
    Resource('target-https-proxies', None, None, None),
    Resource('url-maps', None, None, None),
    Resource('backend-services', None, 'region', None),
    Resource('target-pools', None, 'region', None),
    Resource('health-checks', None, None, None),
    Resource('http-health-checks', None, None, None),
    Resource('instance-groups', None, 'zone', 'Yes'),
    Resource('instance-groups', None, 'zone', 'No'),
    Resource('instance-templates', None, None, None),
    Resource('networks', 'subnets', 'region', None),
    Resource('networks', None, '', None),
]


def collect(project, age, resource, filt):
    """ Collect a list of resources for each condition (zone or region).

    Args:
        project: The name of a gcp project.
        age: Time cutoff from the creation of a resource.
        resource: Definition of a type of gcloud resource.
        filt: Filter clause for gcloud list command.
    Returns:
        A dict of condition : list of gcloud resource object.
    Raises:
        ValueError if json result from gcloud is invalid.
    """

    col = collections.defaultdict(list)

    cmd = ['gcloud', 'compute', '-q', resource.name]
    if resource.group:
        cmd.append(resource.group)
    cmd.extend([
        'list',
        '--format=json(name,creationTimestamp.date(tz=UTC),zone,region,isManaged)',
        '--filter=%s' % filt,
        '--project=%s' % project])
    print '%r' % cmd

    for item in json.loads(subprocess.check_output(cmd)):
        print '%r' % item

        if 'name' not in item or 'creationTimestamp' not in item:
            raise ValueError('%r' % item)

        if resource.condition and resource.condition in item:
            colname = item[resource.condition]
        else:
            colname = ''

        if resource.managed:
            if 'isManaged' not in item:
                raise ValueError(resource.name, resource.managed)
            else:
                if resource.managed != item['isManaged']:
                    continue

        # Unify datetime to use utc timezone.
        created = datetime.datetime.strptime(item['creationTimestamp'], '%Y-%m-%dT%H:%M:%S')
        print ('Found %r(%r), %r in %r, created time = %r' %
               (resource.name, resource.group, item['name'], colname, item['creationTimestamp']))
        if created < age:
            print ('Added to janitor list: %r(%r), %r' %
                   (resource.name, resource.group, item['name']))
            col[colname].append(item['name'])
    return col


def clear_resources(project, cols, resource):
    """Clear a collection of resource, from collect func above.

    Args:
        project: The name of a gcp project.
        cols: A dict of collection of resource.
        resource: Definition of a type of gcloud resource.
    Returns:
        0 if no error
        1 if deletion command fails
    """
    err = 0
    for col, items in cols.items():
        if ARGS.dryrun:
            print ('Resource type %r(%r) to be deleted: %r' %
                   (resource.name, resource.group, list(items)))
            continue

        manage_key = {'Yes':'managed', 'No':'unmanaged'}

        # construct the customized gcloud commend
        base = ['gcloud', 'compute', '-q', resource.name]
        if resource.group:
            base.append(resource.group)
        if resource.managed:
            base.append(manage_key[resource.managed])
        base.append('delete')
        base.append('--project=%s' % project)

        if resource.condition:
            if col:
                base.append('--%s=%s' % (resource.condition, col))
            else:
                base.append('--global')

        print 'Call %r' % base
        try:
            subprocess.check_call(base + list(items))
        except subprocess.CalledProcessError as exc:
            err = 1
            print >>sys.stderr, 'Error try to delete resources: %r' % exc
    return err


def main(project, days, hours, filt):
    """ Clean up resources from a gcp project based on it's creation time

    Args:
        project: The name of a gcp project.
        days/hours: days/hours of maximum lifetime of a gcp resource.
        filt: Resource instance filters when query.
    Returns:
        0 if no error
        1 if list or delete command fails
    """

    print '[=== Start Janitor on project %r ===]' % project
    err = 0
    age = datetime.datetime.utcnow() - datetime.timedelta(days=days, hours=hours)
    for res in DEMOLISH_ORDER:
        print 'Try to search for %r with condition %r' % (res.name, res.condition)
        try:
            col = collect(project, age, res, filt)
            if col:
                err |= clear_resources(project, col, res)
        except ValueError:
            err |= 1 # keep clean the other resource
            print >>sys.stderr, 'Fail to list resource %r from project %r' % (res.name, project)

    print '[=== Finish Janitor on project %r with status %r ===]' % (project, err)
    sys.exit(err)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        description='Clean up resources from an expired project')
    PARSER.add_argument('--project', help='Project to clean', required=True)
    PARSER.add_argument(
        '--days', type=int,
        help='Clean items more than --days old (added to --hours)')
    PARSER.add_argument(
        '--hours', type=float,
        help='Clean items more than --hours old (added to --days)')
    PARSER.add_argument(
        '--filter',
        default='NOT tags.items:do-not-delete AND NOT name ~ ^default',
        help='Filter down to these instances')
    PARSER.add_argument(
        '--dryrun',
        default=False,
        action='store_true',
        help='list but not delete resources')
    ARGS = PARSER.parse_args()

    # We want to allow --days=0 and --hours=0, so check against None instead.
    if ARGS.days is None and ARGS.hours is None:
        print >>sys.stderr, 'must specify --days and/or --hours'
        sys.exit(1)

    main(ARGS.project, ARGS.days or 0, ARGS.hours or 0, ARGS.filter)
