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
    # pylint: disable=too-many-branches
    with open(test_infra('jobs/config.json'), 'r+') as fp:
        configs = json.loads(fp.read())
    regexp = re.compile(r'JENKINS_USE_GCI_VERSION=y|JENKINS_GCI_HEAD_IMAGE_FAMILY=(.+)')
    problems = []
    for job, values in configs.items():
        if values.get('scenario') != 'kubernetes_e2e':
            continue
        migrated = any('--extract=' in a for a in values.get('args', []))
        with open(test_infra('jobs/%s.env' % job)) as fp:
            env = fp.read()
        if migrated:
            if 'JENKINS_USE_GCI_VERSION=' in env or 'JENKINS_GCI_HEAD_IMAGE_FAMILY=' in env:
                problems.append(job)
                continue
        gci = family = None
        lines = []
        for line in env.split('\n'):
            mat = regexp.search(line)
            if not mat:
                lines.append(line)
                continue
            if family and gci:
                print >>sys.stderr, 'Duplicated:', job, line
                problems.append(job)
                break
            if mat.group(1):
                family = mat.group(1)
            else:
                gci = True
        else:
            if bool(gci) ^ bool(family):
                problems.append(job)
                continue
            if family:
                with open(test_infra('jobs/%s.env' % job), 'w') as fp:
                    fp.write('\n'.join(lines))
                values['args'].append('--extract=gci/%s' % family)
    with open(test_infra('jobs/config.json'), 'w') as fp:
        fp.write(json.dumps(configs, sort_keys=True, indent=2))
        fp.write('\n')
    if not problems:
        sys.exit(0)
    print >>sys.stderr, '%d problems' % len(problems)
    print '\n'.join(problems)

if __name__ == '__main__':
    sort()
