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

"""Check the iam rights of test projects, optionally fixing them

Usage:
  python update_jobs.py <config|--default> [--fix]

Config format: {<ROLE>: [MEMBER, ...]}
  {
    "roles/editor": [
      "serviceAccount:foo@bar.com",
      "user:me@you.com",
      "group:whatever@googlegroups.com"],
    "roles/viewer": [...],
    ...
  }
"""


import argparse
import collections
import json
import os.path
import re
import subprocess
import sys
import threading

semaphore = threading.Semaphore(10)  # 10 concurrent gcloud calls
lock = threading.Lock()  # Don't print/append concurrently
null = open('/dev/null', 'w')

# Not thread safe, acquire lock before accessing.
completed = set()  # Projects with no updated needed
updated = set()  # Projects which changed
errors = set()  # Projects which failed to fully update
helpers = {}  # People with owners rights to update error projects

DEFAULT = {
    'roles/editor': [
        'serviceAccount:kubekins@kubernetes-jenkins.iam.gserviceaccount.com',
    ],
}


def GetConfig(string):
  if not string:
    return DEFAULT
  elif not os.path.isfile(string):
    raise argparse.ArgumentTypeError('not a file: %s' % string)
  with open(string) as fp:
    return json.loads(fp.read())


def CheckProjects(needed, job_filename_filter, fix=False):
  projects = LoadProjects(os.path.dirname(__file__) or '.',
                          job_filename_filter)
  print >>sys.stderr, 'Checking %d projects for iam bindings:' % len(projects)
  for role, needs in sorted(needed.items()):
    print >>sys.stderr, '  %s: %s' % (role, ','.join(Sane(n) for n in needs))

  threads = [
      threading.Thread(target=Check, args=(p, needed, fix))
      for p in sorted(projects)
  ]

  for thread in threads:
    thread.start()

  for thread in threads:
    thread.join()

  print >>sys.stderr, '%d already configured, %d updated, %d problems' % (
      len(completed), len(updated), len(errors))

  fixers = collections.defaultdict(int)
  unk = ['user:unknown']
  for project in errors:
    for name in helpers.get(project,unk):
      fixers[name] += 1
    print >>sys.stderr, '  %s: %s' % (
        project, ','.join(Sane(s) for s in sorted(helpers.get(project, unk))))

  print >>sys.stderr, 'Helpers:'
  for name, count in sorted(fixers.items(), key=lambda i: i[1]):
    print >>sys.stderr, '  %s: %s' % (count, Sane(name))
  sys.exit(1)


def Sane(member):
  if ':' not in member:
    raise ValueError(member)
  email = member.split(':')[1]
  return email.split('@')[0]


def LoadProjects(configs, job_filename_filter):
  projects = set()
  for dirname, _, files in os.walk(os.path.dirname(__file__) or '.'):
    for path in files:
      full_path = os.path.join(dirname, path)
      if not job_filename_filter in full_path or not path.endswith('.yaml'):
        continue
      with open(full_path) as fp:
        for project in re.findall(r'PROJECT="(.+)"', fp.read()):
          if '{' not in project:
            projects.add(project)
            continue
          elif '{version-infix}' not in project:
            with lock:
              print >>sys.stderr, 'project expansion not allowed ',
              print project
              errors.add(project)
          for version in [
              '1-0-1-2',
              '1-1-1-2',
              '1-1-1-3',
              '1-2-1-3',
              '1-2-1-4',
              '1-3-1-4',
          ]:
            projects.add(project.replace('{version-infix}', version))
  return projects


def Update(project, role, member):
  with semaphore:
    err = subprocess.call(
        [
            'gcloud', '-q', 'projects',
            'add-iam-policy-binding',
            '--role=%s' % role,
            '--member=%s' % member,
            project,
        ],
        stdout=null,
    )

  if not err:
    with lock:
      print >>sys.stderr, 'Added %s as %s to %s' % (member, role, project)
      updated.add(project)
      return

  with lock:
    print >>sys.stderr, 'could not update ',
    print project
    errors.add(project)


def Check(project, needed, mutate):
  try:
    with semaphore:
      out = subprocess.check_output([
          'gcloud',
          'projects',
          'get-iam-policy',
          project,
          '--format=json(bindings)',
      ])
  except subprocess.CalledProcessError:
    with lock:
      print >>sys.stderr, 'cannot access ',
      print project
      errors.add(project)
      return

  bindings = json.loads(out)
  fixes = {}
  for binding in bindings['bindings']:
    role = binding['role']
    members = binding['members']
    if role in needed:
      missing = set(needed[role]) - set(members)
      if missing:
        fixes[role] = missing
    if role == 'roles/owner':
      with lock:
        helpers[project] = members

  if not fixes:
    with lock:
      print >>sys.stderr, project, 'already configured'
      completed.add(project)
      return

  if not mutate:
    with lock:
      print >>sys.stderr, 'Will not --fix ',
      print project
      print >>sys.stderr, '  wanted fixes: ', fixes
      errors.add(project)
      return

  updates = []
  for role, members in sorted(fixes.items()):
    updates.extend(
        threading.Thread(target=Update, args=(project, role, m))
        for m in members
    )

  for update in updates:
    update.start()

  for update in updates:
    update.join()


if __name__ == '__main__':
  parser = argparse.ArgumentParser(description=__doc__)
  parser.add_argument(
      '--fix', action='store_true', help='Add missing memberships')
  parser.add_argument(
      'config', type=GetConfig, default='', nargs='?', help='Path to json configuration')
  parser.add_argument(
      '--job_filename_filter', default='',
      help='Only look for projects in job YAML paths containing this string')
  args = parser.parse_args()
  CheckProjects(args.config, args.job_filename_filter, args.fix)
