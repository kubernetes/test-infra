#!/usr/bin/env python

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

"""Migrate --extract flag from an JENKINS_FOO env to a scenario flag."""

import json
import os
import re
import sys

ORIG_CWD = os.getcwd()  # Checkout changes cwd

def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)

def sort():
    """Sort config.json alphabetically."""
    # pylint: disable=too-many-branches,too-many-statements,too-many-locals
    with open(test_infra('jobs/config.json'), 'r+') as fp:
        configs = json.loads(fp.read())
    regexp = re.compile('|'.join([
        r'^KUBE_OS_DISTRIBUTION=(.*)$',
    ]))
    problems = []
    for job, values in configs.items():
        if values.get('scenario') != 'kubernetes_e2e':
            continue
        if 'args' not in values:
            continue
        args = values['args']
        if any('None' in a for a in args):
            problems.append('Bad flag with None: %s' % job)
            continue

        if not os.path.isfile(test_infra('jobs/env/%s.env' % job)):
            continue
        with open(test_infra('jobs/env/%s.env' % job)) as fp:
            env = fp.read()
        lines = []
        mod = False
        os_image = ''
        for line in env.split('\n'):
            mat = regexp.search(line)
            if mat:
                os_image = mat.group(1)
                mod = True
                continue
            lines.append(line)
        if not mod:
            continue

        args = list(args)
        if os_image:
            args.append('--gcp-node-image=%s' % os_image)
            args.append('--gcp-master-image=%s' % os_image)
        flags = set()
        okay = False
        for arg in args:
            try:
                flag, _ = arg.split('=', 1)
            except ValueError:
                flag = ''
            if flag and flag not in ['--env-file', '--extract']:
                if flag in flags:
                    problems.append('Multiple %s in %s' % (flag, job))
                    break
                flags.add(flag)
        else:
            okay = True
        if not okay:
            continue
        values['args'] = args
        with open(test_infra('jobs/env/%s.env' % job), 'w') as fp:
            fp.write('\n'.join(lines))
    with open(test_infra('jobs/config.json'), 'w') as fp:
        fp.write(json.dumps(configs, sort_keys=True, indent=2, separators=(',', ': ')))
        fp.write('\n')
    if not problems:
        sys.exit(0)
    print >>sys.stderr, '%d problems' % len(problems)
    print '\n'.join(problems)

if __name__ == '__main__':
    sort()
