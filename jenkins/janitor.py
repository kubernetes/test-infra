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
import re
import subprocess
import sys

# A resource that need to be cleared.
Resource = namedtuple('Resource', 'name condition status')
demolish_order = [
Resource("instances", "zone", None),
Resource("addresses", "region", None),
Resource("disks", "zone", None),
Resource("firewall-rules", None, None),
Resource("routes", None, None),
Resource("forwarding-rules", "region", None),
Resource("target-pools", "region", None),
Resource("instance-groups", "zone", "managed"),
Resource("instance-templates", None, None),
# Beaware of insertion order
]

def Collect(project, age, resource, filt):
    col = collections.defaultdict(list)

    if not resource.condition:
        condclause = '--format=value(name,creationTimestamp),json'
    else:
        condclause = '--format=value(name,creationTimestamp,%s),json'\
                     % resource.condition

    for item in json.loads(subprocess.check_output([
     'gcloud', 'compute', '-q',
     resource.name, 'list',
     condclause,
     '--filter=%s' % filt,
     '--project=%s' % project]).split('\n')):
        
        if not item['name'] or not item['creationTimestamp']:
            continue
        if resource.condition and not item[resource.condition]:
            continue

        colname = item[resource.condition] or ""
        tfmt = 'YYYY-mm-ddTHH:MM:SS'
        created = datetime.datetime.strptime(item['creationTimestamp'][:len(tfmt)],
                                             '%Y-%m-%dT%H:%M:%S')
        print "Found %s, %s in %s" % (resource.name, item['name'], colname)
        if created < age:
            print "Include %s, %s" % (resource.name, item['name'])
            col[colname].append(item['name'])
    return col


def ClearResources(project, col, resource):
    err = 0
    for col, items in col.items():
        # construct the customized gcloud commend
        base = ['gcloud', 'compute', '-q', resource.name]
        if resource.status is not None:
            base.append(resource.status)
        base.append('delete')
        base.append('--project=%s' % project)
        if resource.condition is not None:
            base.append('--%s=%s' % (resource.condition, col))

        print "Try to kill %s - %s" % (col, list(items))
        if subprocess.call(base + list(items)) is not 0:
           err = 1
           print "Error try to delete %s - %s" % (col, list(items))
    return err


def main(project, days, hours, filt):
    age = datetime.datetime.now() - datetime.timedelta(days=days, hours=hours)
    err = 0
    for r in demolish_order:
        print "Try to search for %s with condition %s" % (r.name, r.condition)
        col = Collect(project, age, r, filt)
        if col:
            err |= ClearResources(project, col, r)
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

