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
        r'E2E_UPGRADE_TEST=true',
        r'GINKGO_UPGRADE_TEST_ARGS=(.+)',
    ]))
    problems = []
    for job, values in configs.items():
        if values.get('scenario') != 'kubernetes_e2e':
            continue
        migrated = any('--upgrade_args=' in a for a in values.get('args', []))
        with open(test_infra('jobs/%s.env' % job)) as fp:
            env = fp.read()
        if migrated:
            if any(j in env for j in [
                    'GINKGO_UPGRADE_TEST_ARGS=',
                    'E2E_UPGRADE_TEST=',
            ]):
                problems.append(job)
            continue
        upgrading = False
        upgrade_args = None
        lines = []
        for line in env.split('\n'):
            mat = regexp.search(line)
            if not mat:
                lines.append(line)
                continue
            args = mat.group(1)
            if args:
                if upgrade_args:
                    problems.append('Duplicate %s' % job)
                    break
                upgrade_args = args
                continue
            if upgrading:
                problems.append('Duplicate E2E_UPGRADE_TEST in %s' % job)
                break
            else:
                upgrading = True
        else:
            if not upgrading:
                continue
            if upgrading and not args:
                problems.append('Missing GINKGO_UPGRADE_TEST_ARGS in %s' % job)
                continue
            values['args'].append('--upgrade_args=%s' % upgrade_args)
            with open(test_infra('jobs/%s.env' % job), 'w') as fp:
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
