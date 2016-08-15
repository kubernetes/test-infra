#!/usr/bin/env python

# Copyright 2016 The Kubernetes Authors All rights reserved.
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
import re
import subprocess
import sys


def Instances(project, age, filt):
  zones = collections.defaultdict(list)
  for inst in subprocess.check_output([
      'gcloud', 'compute', '-q',
      'instances', 'list',
      '--format=value(name,zone,creationTimestamp)',
      '--filter=%s' % filt,
      '--project=%s' % project]).split('\n'):
    inst = inst.strip()
    if not inst:
      continue
    name, zone, created_str = re.split(r'\s+', inst)
    tfmt = 'YYYY-mm-ddTHH:MM:SS'
    created = datetime.datetime.strptime(created_str[:len(tfmt)], '%Y-%m-%dT%H:%M:%S')
    if created < age:
      zones[zone].append(name)
  return zones


def DeleteInstances(project, zones, delete):
  err = 0
  for zone, instances in zones.items():
    base = [
        'gcloud', 'compute', '-q',
        'instances', 'delete',
        '--project=%s' % project,
        '--zone=%s' % zone
    ]
    if not delete:
      print >>sys.stderr, '--delete will run the following:'
      base.insert(0, 'echo')
    err |= subprocess.call(base + list(instances))
  return err


def main(project, hours, filt, delete):
  age = datetime.datetime.now() - datetime.timedelta(hours=int(hours))
  zones = Instances(project, age, filt)
  if zones:
    sys.exit(DeleteInstances(project, zones, delete))


if __name__ == '__main__':
  parser = argparse.ArgumentParser(
      description='Delete old instances from a project')
  parser.add_argument('--project', help='Project to clean', required=True)
  parser.add_argument(
      '--hours', type=float, help='Clean items more than --hours old', required=True)
  parser.add_argument(
      '--delete', action='store_true', help='Really delete things when set')
  parser.add_argument(
      '--filter', default='name:tmp*', help='Filter down to these instances')
  args = parser.parse_args()
  main(args.project, args.hours, args.filter, args.delete)
