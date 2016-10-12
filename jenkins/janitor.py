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

import argparse
import collections
import datetime
import json
import pytz
import subprocess
import sys

from collections import namedtuple
from dateutil import parser as dateparser


# A resource that need to be cleared.
Resource = namedtuple('Resource', 'name condition status')
DEMOLISH_ORDER = [
    # Beaware of insertion order
    Resource("instances", "zone", None),
    Resource("addresses", "region", None),
    Resource("disks", "zone", None),
    Resource("firewall-rules", None, None),
    Resource("routes", None, None),
    Resource("forwarding-rules", "region", None),
    Resource("target-pools", "region", None),
    Resource("instance-groups", "zone", "managed"),
    Resource("instance-templates", None, None),
]


def collect(project, age, resource, filt):
    """ Collect a list of resources for each condition (zone or region).

    Args:
        project: The name of a gcp project.
        age: Time cutoff from the creation of a resource.
        resource: Definition of a type of gcloud resource.
        filt: Filter clause for gcloud list command.
    Returns:
        A dict of condition : list of resource.
    """

    col = collections.defaultdict(list)

    for item in json.loads(subprocess.check_output([
            'gcloud', 'compute', '-q',
            resource.name, 'list',
            '--format=json',
            '--filter=%s' % filt,
            '--project=%s' % project])):

        if not item['name'] or not item['creationTimestamp']:
            print "[Warning] - Skip item without name or timestamp"
            print "%s" % item
            continue
        if resource.condition and not item[resource.condition]:
            print "[Warning] - condition specified but not found"
            continue

        if resource.condition:
            colname = item[resource.condition]
        else:
            colname = ""

        # Unify datetime to use utc timezone.
        created = dateparser.parse(item['creationTimestamp']).astimezone(pytz.utc)
        print "Found %s, %s in %s" % (resource.name, item['name'], colname)
        if created < age:
            print "Include %s, %s" % (resource.name, item['name'])
            col[colname].append(item['name'])
    return col


def clear_resources(project, col, resource):
    """Clear a collection of resource, from collect func above.

    Args:
        project: The name of a gcp project.
        col: A dict of collection of resource.
        resource: Definition of a type of gcloud resource.
    Returns:
        0 if no error
        1 if deletion command fails
    """
    err = 0
    for col, items in col.items():
        # construct the customized gcloud commend
        base = ['gcloud', 'compute', '-q', resource.name]
        if resource.status is not None:
            base.append(resource.status)
        base.append('delete')
        base.append('--project=%s' % project)
        if resource.condition:
            base.append('--%s=%s' % (resource.condition, col))

        print "Try to kill %s - %s" % (col, list(items))
        if subprocess.call(base + list(items)) != 0:
            err = 1
            print "Error try to delete %s - %s" % (col, list(items))
    return err


def main(project, days, hours, filt):
    err = 0
    age = datetime.datetime.utcnow() - datetime.timedelta(days=days, hours=hours)
    age = age.replace(tzinfo=pytz.utc)
    for r in DEMOLISH_ORDER:
        print "Try to search for %s with condition %s" % (r.name, r.condition)
        col = collect(project, age, r, filt)
        if col:
            err |= clear_resources(project, col, r)
    sys.exit(err)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(
        description='Clean up resources from an expired project')
    parser.add_argument('--project', help='Project to clean', required=True)
    parser.add_argument(
        '--days', type=int,
        help='Clean items more than --days old (added to --hours)')
    parser.add_argument(
        '--hours', type=float,
        help='Clean items more than --hours old (added to --days)')
    parser.add_argument(
        '--filter',
        default="NOT tags.items:do-not-delete AND NOT name ~ ^default-route",
        help='Filter down to these instances')
    args = parser.parse_args()

    # We want to allow --days=0 and --hours=0, so check against None instead.
    if args.days is None and args.hours is None:
        print >>sys.stderr, 'must specify --days and/or --hours'
        sys.exit(1)

    main(args.project, args.days or 0, args.hours or 0, args.filter)

