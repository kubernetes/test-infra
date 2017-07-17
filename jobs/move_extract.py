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
        r'^E2E_MIN_STARTUP_PODS=(.*)$',
        r'^E2E_CLEAN_START=(true)$',
        r'^CLUSTER_IP_RANGE=(.*)$',
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
        with open(test_infra('jobs/%s.env' % job)) as fp:
            env = fp.read()
        lines = []
        found = {}
        def check(key, val):
            # pylint: disable=cell-var-from-loop
            if not val:
                return False
            if key in found:
                problems.append('Duplicate %s in %s' % (key, job))
                return True
            found[key] = val
            return False
        for line in env.split('\n'):
            mat = regexp.search(line)
            if not mat:
                lines.append(line)
                continue
            pods, clean, ip_range = mat.groups()
            if check('--minStartupPods', pods):
                break
            if check('--clean-start', clean):
                break
            if check('--cluster-ip-range', ip_range):
                break
        else:
            stop = False
            for arg in args:
                if '--minStartupPods' in found:
                    break
                if arg == '--env-file=jobs/pull-kubernetes-e2e.env':
                    if check('--minStartupPods', '1'):
                        stop = True
                        break
                if arg == '--env-file=jobs/platform/gce.env':
                    if check('--minStartupPods', '8'):
                        stop = True
                        break
                if arg == '--env-file=jobs/platform/gke.env':
                    if check('--minStartupPods', '8'):
                        stop = True
                        break
            if stop:
                continue

            new_args = []
            for arg in args:
                if not arg.startswith('--test_args='):
                    new_args.append(arg)
                    continue
                _, val = arg.split('=', 1)
                vals = [val]
                for key, val in found.items():
                    vals.append('%s=%s' % (key, val))
                new_args.append('--test_args=%s' % ' '.join(vals))
            values['args'] = new_args
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
