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
        r'^GINKGO_TEST_ARGS=(.*)$|^SKEW_KUBECTL=(y)$'
    ]))
    problems = []
    for job, values in configs.items():
        if values.get('scenario') != 'kubernetes_e2e':
            continue
        if 'args' not in values:
            continue
        args = values['args']
        new_args = [a for a in args if a != '--test_args=None']
        if new_args != args:
            args = new_args
            values['args'] = args
        if any('None' in a for a in args):
            problems.append('Bad flag with None: %s' % job)
            continue
        if any(a.startswith('--test_args=') for a in args):
            continue
        with open(test_infra('jobs/%s.env' % job)) as fp:
            env = fp.read()
        tests = None
        skew = False
        lines = []
        for line in env.split('\n'):
            mat = regexp.search(line)
            if not mat:
                lines.append(line)
                continue
            group, now_skew = mat.groups()
            if group:
                if tests:
                    problems.append('Duplicate %s' % job)
                    break
                tests = group
                continue
            if now_skew:
                if skew:
                    problems.append('Duplicate skew %s' % job)
                skew = now_skew
        else:
            new_args = []
            stop = False
            for arg in args:
                these = None
                add = True
                if (
                        arg == '--env-file=jobs/pull-kubernetes-federation-e2e-gce.env'
                        and not job == 'pull-kubernetes-federation-e2e-gce'):
                    these = r'--ginkgo.skip=\[Slow\]|\[Serial\]|\[Disruptive\]|\[Flaky\]|\[Feature:.+\]'  # pylint: disable=line-too-long
                elif (
                        arg == '--env-file=jobs/pull-kubernetes-e2e.env'
                        and not job.startswith('pull-kubernetes-federation-e2e-gce')):
                    these = r'--ginkgo.skip=\[Slow\]|\[Serial\]|\[Disruptive\]|\[Flaky\]|\[Feature:.+\]'  # pylint: disable=line-too-long
                elif arg == '--env-file=jobs/suite/slow.env':
                    these = r'--ginkgo.focus=\[Slow\] --ginkgo.skip=\[Serial\]|\[Disruptive\]|\[Flaky\]|\[Feature:.+\]'  # pylint: disable=line-too-long
                elif arg == '--env-file=jobs/suite/serial.env':
                    these = r'--ginkgo.focus=\[Serial\]|\[Disruptive\] --ginkgo.skip=\[Flaky\]|\[Feature:.+\]'  # pylint: disable=line-too-long
                    add = False
                elif arg == '--env-file=jobs/suite/default.env':
                    these = r'--ginkgo.skip=\[Slow\]|\[Serial\]|\[Disruptive\]|\[Flaky\]|\[Feature:.+\]'  # pylint: disable=line-too-long
                if add:
                    new_args.append(arg)
                if not these:
                    continue
                if tests:
                    problems.append('Duplicate end %s' % job)
                    stop = True
                    break
                tests = these
            if stop:
                continue
            args = new_args

            testing = '--test=false' not in args

            if not testing:
                if skew:
                    problems.append('Cannot skew kubectl without tests %s' % job)
                if tests:
                    problems.append('Cannot --test_args when --test=false %s' % job)
                continue
            if skew:
                path = '--kubectl-path=../kubernetes_skew/cluster/kubectl.sh'
                if tests:
                    tests = '%s %s' % (tests, path)
                else:
                    tests = path
            if tests:
                args.append('--test_args=%s' % tests)
            values['args'] = args
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
