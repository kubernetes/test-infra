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

"""Delete this code as soon as possible and/or convert to yaml linter.

This is a hacky file designed to extract job-env from jobs in JJB.
"""


import os
import pprint
import re
import sys

import yaml


class Context(dict):
  """A stack of contexts.

  empty = Context({})
  a = Context(empty, {'a': 1})
  b = Context(a, {'b': 2})
  c = Context(b, {'c': 3})
  context = Context(c)
  assert context['a'] == a['a']
  b['a'] = 5
  assert context['a'] == 5
  """

  def __init__(self, base, *a, **kw):
    super(Context, self).__init__(*a, **kw)
    self.base = base

  def __getitem__(self, item):
    if super(Context, self).__contains__(item):
      return super(Context, self).__getitem__(item)
    return self.base[item]

  def __contains__(self, item):
    if super(Context, self).__contains__(item):
      return True
    if self.base is not None:
      return item in self.base
    return False


# Make sure provider-env is one of these values
PROVIDERS = [
    'aws',
    'gce',
    'gke',
]


def CheckContext(name, context):
  # Each job must specify a provider-env
  if 'provider-env' not in context:
    raise ValueError('missing provider-env', name, context)
  if context['provider-env'] not in PROVIDERS:
    raise ValueError('bad provider', name, context['provider-env'])

  # Each job must have test-infra in its workspace
  if 'scm' not in context:
    raise ValueError('Workspace must export kubernetes/test-infra', name)
  infra = False
  for scm in context['scm']:
    if scm.get('git')['url'].endswith('/test-infra'):
      # Basedir must be the the following:
      if not scm['git']['basedir'] == 'go/src/k8s.io/test-infra':
        raise ValueError('bad test-infra basedir', name, scm)
      infra |= True
  if not infra:
    raise ValueError('Workspace must export kubernetes/test-infra', name, context['scm'])

  # Must shell out the following lines:
  expected = {
      'provider': 'export KUBEKINS_PROVIDER_ENV="{provider-env}.env"\n',
      'job': 'export KUBEKINS_JOB_ENV="{job-name}.env"\n',
      #'runner': '{runner}',
  }
  # Must not shell out these lines
  unexpected = {
      'provider-env': '{provider-env}\n',
      'job-env': '{job-env}\n',
  }
  found = set()
  bad_found = set()
  for builder in context['builders']:
    if not isinstance(builder, dict):
      continue
    value = builder.get('shell', '')
    for key, check in expected.items():
      if key in found:
        continue
      if check in value:
        found.add(key)
    for key, check in unexpected.items():
      if key in bad_found:
        continue
      if check in value:
        bad_found.add(key)

  missing = set(expected) - found
  if missing:
    raise ValueError(name, [expected[m] for m in missing])
  if bad_found:
    raise ValueError(name, [unexpected[b] for b in bad_found])


def ConvertJob(context, out_path):
  """Extract job-env from a context, including any {replacements}."""
  name = context['name']

  # Ensure job-env is set or else it already has a .env file
  path = os.path.join(out_path, '%s.env' % name)
  if 'job-env' not in context:
    if not os.path.isfile(path):
      raise ValueError(name, path)
    print >>sys.stderr, '  CONFIGURED:', path
    return

  # Check the sanity of the context
  CheckContext(name, context)

  # Replace job-env variables as necessary.
  print >>sys.stderr, '  ', path
  for resolved in Resolve(context, 'job-env'):
    parsed = []
    # Put each variable on a single line
    # Aka convert this:
    #   export FOO="spam \
    #               eggs"
    # to this:
    #   export FOO="spam eggs"
    for env in re.sub(r'\\\n\s*', '', resolved['job-env']).split('\n'):
      # Ensure everything starts with export or is a comment
      if env and not (env.startswith('export') or env.startswith('#')):
        raise ValueError(env)
      # Convert export FOO="bar" to FOO=bar
      parsed.append(env.replace('"','').replace('export ',''))
    for p in parsed:
      print >>sys.stderr, '  ', p
    with open(path, 'w') as fp:
      fp.write('\n'.join(parsed))
    print >>sys.stderr, '  WROTE: %s (delete job-env)' % path


class JJBObject(dict):
  def __init__(self, kind, name, values):
    super(JJBObject, self).__init__(values)
    self.kind = kind
    self.name = name


def KindList(items):
  """A list of kinds (project, job-template) with a name field.

  The following yaml:
  - project:
      name: foo
  - project:
      name: bar
  - job-template:
      name: hello
  Should result in:
    [JJObject('project', 'foo', {'name': 'foo'}),
     JJObject('project', 'bar', {'name': 'bar'}),
     JJObject('job-template', 'hello', {'name': 'hello'}]
  """
  for item in items:
    if not isinstance(item, dict):
      raise ValueError('jbb objects are dicts', item)
    if len(item) != 1:
      raise ValueError('jjb objects have one item', item)
    kind = item.keys()[0]
    value = item[kind]
    if 'name' not in value:
      raise ValueError('missing name key', item)
    yield JJBObject(kind, value['name'], value)


def NameList(kind, items):
  """A list of names of objects (jobs) of the same kind.

  The following yaml:
    - 'kubernetes-e2e-{suffix}'
    - 'kubernetes-e2e-{gke-suffix}':
        argle: bargle
    - 'boring':
        spam: eggs

  Should result in:
    JJObject(kind, 'kubernetes-e2e-{suffix}', {})
    JJObject(kind, 'kubernetes-e2e-{gke-suffix}', {'argle': 'bargle'})
    JJObject(kind, 'boring', {'spam': 'eggs'})
  """
  for item in items:
    if isinstance(item, basestring):
      yield JJBObject(kind, item, {})
      continue

    if not isinstance(item, dict):
      raise ValueError('jbb objects are dicts', item)
    if len(item) != 1:
      raise ValueError('jjb objects have one item', item)
    name = item.keys()[0]
    value = item[name]
    yield JJBObject(kind, name, value or {})


def Jobs(item, context, jjb_objects):
  context = Context(context, item)
  # job-template, resolve the name and yield the resulting context
  if item.kind not in ['project', 'job-group']:
    for resolved in Resolve(context, 'name'):
      yield resolved
    return

  # We need to expand the jobs for project or job-group
  if 'jobs' not in item:
    raise ValueError('Empty', item.kind, item.name)
  jobs = item['jobs']
  # Lookup each job name, expanding as necessary
  for ref in NameList('unknown', jobs):
    if ref.name not in jjb_objects:
      raise ValueError('Cannot find %s in %s' % (ref.name, item.name))
    obj = jjb_objects[ref.name]  # Real object
    for resolved in Jobs(obj, Context(context, ref), jjb_objects):
      yield resolved


def Resolve(context, key):
  """Resolve any {variables} in the context, yielding the result.

  context = Context({}, {
      'target': 'this is {really} {annoying}',
      'really': 'super duper',
      'annoying': 'fun and exciting'})
  assert (
      Resolve(context, 'target')['target'] ==
      'this is really super duper fun and exciting')
  """
  replacements = re.findall(r'{([^}]+)}', context[key])
  if not replacements:
    yield context  # Nothing to do, yay!
    return

  # Replace the first {variable} in context[key] with context[variable]
  repl = replacements[0]
  val = context[repl]
  if isinstance(val, basestring):
    val = [val]
  for val in NameList('unknown', val):
    val[key] = context[key].replace('{%s}' % repl, val.name)
    for resolved in Resolve(Context(context, val), key):
      yield resolved

# Some files have a mix of e2e and other jobs
# Skip specific jobs in these files
SKIP_JOBS = [
    'continuous-node-e2e-docker-validation-gci',
]


def ParseFile(path, out_path):
  """Extract every job's job-env in path to out_path/<job-name>.env"""
  print 'Processing %s...' % path
  with open(path) as fp:
    buf = fp.read()

  root = yaml.safe_load(buf)
  jjb_objects = {}
  projects = {}
  for obj in KindList(root):
    if obj.name in jjb_objects:
      raise ValueError('Duplicate name', name)
    jjb_objects[obj.name] = obj
    if obj.kind == 'project':
      projects[obj.name] = obj

  for name in projects:
    project = projects[name]
    context = {}
    for resolved in Jobs(project, context, jjb_objects):
      if resolved['name'] in SKIP_JOBS:
        print >>sys.stderr, 'Skipping %s...' % resolved['name']
        continue
      print project.name, resolved['name']
      ConvertJob(resolved, out_path)


# Many files do not use {runner}/{job-env}
SKIP = [
    'cloud-resource-cleanup.yaml',  # job not defined in file
    'kops-build.yaml',  # no job-env
    'kubernetes-build.yaml',  # no job-env
    'kubernetes-djmm.yaml',  # no job-env
    'kubernetes-test-go.yaml',  # no job-env
    'kubernetes-verify.yaml',  # no job-env
    'node-e2e.yaml',  # doesn't use job-env
]


def main(
    in_path='./jenkins/job-configs/kubernetes-jenkins',
    out_path='./job-configs'):
  if not os.path.isdir(out_path):
    raise ValueError('not a directory', out_path)
  if os.path.isdir(in_path):
    for dirpath, _, names in os.walk(in_path):
      for name in names:
        if name in SKIP:
          print >>sys.stderr, 'Skipping %s' % name
          continue
        ParseFile(os.path.join(dirpath, name), out_path)
    return
  ParseFile(in_path, out_path)



if __name__ == '__main__':
  main(*sys.argv[1:])
